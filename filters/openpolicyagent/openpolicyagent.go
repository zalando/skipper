package openpolicyagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/singleflight"
	"io"
	"maps"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"google.golang.org/protobuf/proto"

	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/google/uuid"
	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/config"
	"github.com/open-policy-agent/opa/v1/download"
	"github.com/open-policy-agent/opa/v1/hooks"
	"github.com/open-policy-agent/opa/v1/logging"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/plugins/discovery"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/runtime"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	iCache "github.com/open-policy-agent/opa/v1/topdown/cache"
	opatracing "github.com/open-policy-agent/opa/v1/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters/openpolicyagent/internal"
	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/envoy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing"
)

const (
	DefaultCleanIdlePeriod      = 10 * time.Second
	DefaultControlLoopInterval  = 60 * time.Second
	DefaultControlLoopMaxJitter = 1000 * time.Millisecond
	defaultReuseDuration        = 30 * time.Second
	defaultShutdownGracePeriod  = 30 * time.Second
	DefaultOpaStartupTimeout    = 30 * time.Second

	DefaultMaxRequestBodySize    = 1 << 20 // 1 MB
	DefaultMaxMemoryBodyParsing  = 100 * DefaultMaxRequestBodySize
	DefaultRequestBodyBufferSize = 8 * 1024 // 8 KB

	spanNameEval = "open-policy-agent"
)

type BackgroundTask struct {
	fn     func() (interface{}, error)
	done   chan struct{}
	result interface{}
	err    error
	once   sync.Once
}

// Wait blocks until the task completes and returns the result and error
func (t *BackgroundTask) Wait() (interface{}, error) {
	<-t.done
	return t.result, t.err
}

// execute runs the task function and stores the result
func (t *BackgroundTask) execute() {
	t.once.Do(func() {
		defer close(t.done)
		t.result, t.err = t.fn()
	})
}

type InstanceState string

const (
	InstanceStateLoading InstanceState = "loading"
	InstanceStateReady   InstanceState = "ready"
	InstanceStateFailed  InstanceState = "failed"
)

type InstanceInfo struct {
	instance *OpenPolicyAgentInstance
	state    InstanceState
	lastUsed time.Time
	error    error
}

type OpenPolicyAgentRegistry struct {
	// Ideally share one Bundle storage across many OPA "instances" using this registry.
	// This allows to save memory on bundles that are shared
	// between different policies (i.e. global team memberships)
	// This not possible due to some limitations in OPA
	// See https://github.com/open-policy-agent/opa/issues/5707

	mu        sync.Mutex
	instances map[string]*InstanceInfo

	once                   sync.Once
	closed                 bool
	quit                   chan struct{}
	reuseDuration          time.Duration
	cleanInterval          time.Duration
	instanceStartupTimeout time.Duration
	configTemplate         *OpenPolicyAgentInstanceConfig

	maxMemoryBodyParsingSem *semaphore.Weighted
	maxRequestBodyBytes     int64
	bodyReadBufferSize      int64

	tracer opentracing.Tracer

	enableCustomControlLoop bool
	controlLoopInterval     time.Duration
	controlLoopMaxJitter    time.Duration

	enableDataPreProcessingOptimization bool

	valueCache iCache.InterQueryValueCache

	// New fields for pre-loading support
	preloadingEnabled bool

	// Track in-flight instance creation to prevent concurrent creation of the same bundle
	inFlightCreation  map[string]chan *OpenPolicyAgentInstance
	singleflightGroup singleflight.Group

	// Background task system
	backgroundTaskChan   chan *BackgroundTask
	backgroundWorkerOnce sync.Once
}

type OpenPolicyAgentFilter interface {
	OpenPolicyAgent() *OpenPolicyAgentInstance
}

func WithReuseDuration(duration time.Duration) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.reuseDuration = duration
		return nil
	}
}

func WithMaxRequestBodyBytes(n int64) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.maxRequestBodyBytes = n
		return nil
	}
}

func WithMaxMemoryBodyParsing(n int64) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.maxMemoryBodyParsingSem = semaphore.NewWeighted(n)
		return nil
	}
}

func WithReadBodyBufferSize(n int64) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.bodyReadBufferSize = n
		return nil
	}
}

func WithCleanInterval(interval time.Duration) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.cleanInterval = interval
		return nil
	}
}

func WithInstanceStartupTimeout(timeout time.Duration) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.instanceStartupTimeout = timeout
		return nil
	}
}

func WithOpenPolicyAgentInstanceConfig(opts ...func(*OpenPolicyAgentInstanceConfig) error) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		// Create config from registry's default config
		config, err := NewOpenPolicyAgentConfig(opts...)
		if err != nil {
			return err
		}
		cfg.configTemplate = config
		return nil
	}
}

func WithTracer(tracer opentracing.Tracer) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.tracer = tracer
		return nil
	}
}

func WithEnableCustomControlLoop(enabled bool) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.enableCustomControlLoop = enabled
		return nil
	}
}

func WithEnableDataPreProcessingOptimization(enabled bool) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.enableDataPreProcessingOptimization = enabled
		return nil
	}
}

func WithControlLoopInterval(interval time.Duration) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.controlLoopInterval = interval
		return nil
	}
}

func WithControlLoopMaxJitter(maxJitter time.Duration) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.controlLoopMaxJitter = maxJitter
		return nil
	}
}

func (registry *OpenPolicyAgentRegistry) initializeCache() error {
	id := uuid.New().String()
	parsedConfig, err := config.ParseConfig(registry.configTemplate.configTemplate, id)
	if err != nil {
		return fmt.Errorf("failed to parse opa config template: %w", err)
	}
	interQueryBuiltinValueCache, err := iCache.ParseCachingConfig(parsedConfig.Caching)
	if err != nil {
		return err
	}

	registry.valueCache = iCache.NewInterQueryValueCache(context.Background(), interQueryBuiltinValueCache)

	return nil
}

func WithPreloadingEnabled(enabled bool) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.preloadingEnabled = enabled
		return nil
	}
}

func NewOpenPolicyAgentRegistry(opts ...func(*OpenPolicyAgentRegistry) error) (*OpenPolicyAgentRegistry, error) {
	registry := &OpenPolicyAgentRegistry{
		reuseDuration:          defaultReuseDuration,
		cleanInterval:          DefaultCleanIdlePeriod,
		instanceStartupTimeout: DefaultOpaStartupTimeout,
		instances:              make(map[string]*InstanceInfo),
		quit:                   make(chan struct{}),
		maxRequestBodyBytes:    DefaultMaxMemoryBodyParsing,
		bodyReadBufferSize:     DefaultRequestBodyBufferSize,
		controlLoopInterval:    DefaultControlLoopInterval,
		controlLoopMaxJitter:   DefaultControlLoopMaxJitter,
		inFlightCreation:       make(map[string]chan *OpenPolicyAgentInstance),
		backgroundTaskChan:     make(chan *BackgroundTask, 100), // Buffered channel for background tasks
	}

	for _, opt := range opts {
		opt(registry)
	}

	if registry.configTemplate == nil {
		config, err := NewOpenPolicyAgentConfig()

		if err != nil {
			return nil, err
		}

		registry.configTemplate = config
	}

	if registry.maxMemoryBodyParsingSem == nil {
		registry.maxMemoryBodyParsingSem = semaphore.NewWeighted(DefaultMaxMemoryBodyParsing)
	}

	if err := registry.initializeCache(); err != nil {
		return nil, err
	}

	go registry.startCleanerDaemon()

	if registry.enableCustomControlLoop {
		go registry.startCustomControlLoopDaemon()
	}

	return registry, nil
}

type OpenPolicyAgentInstanceConfig struct {
	envoyMetadata  *ext_authz_v3_core.Metadata
	configTemplate []byte
}

func WithConfigTemplate(configTemplate []byte) func(*OpenPolicyAgentInstanceConfig) error {
	return func(cfg *OpenPolicyAgentInstanceConfig) error {
		cfg.configTemplate = configTemplate
		return nil
	}
}

func WithConfigTemplateFile(configTemplateFile string) func(*OpenPolicyAgentInstanceConfig) error {
	return func(cfg *OpenPolicyAgentInstanceConfig) error {
		var err error
		cfg.configTemplate, err = os.ReadFile(configTemplateFile)
		return err
	}
}

func WithEnvoyMetadata(metadata *ext_authz_v3_core.Metadata) func(*OpenPolicyAgentInstanceConfig) error {
	return func(cfg *OpenPolicyAgentInstanceConfig) error {
		cfg.envoyMetadata = metadata
		return nil
	}
}

func WithEnvoyMetadataBytes(content []byte) func(*OpenPolicyAgentInstanceConfig) error {
	return func(cfg *OpenPolicyAgentInstanceConfig) error {
		cfg.envoyMetadata = &ext_authz_v3_core.Metadata{}

		return protojson.Unmarshal(content, cfg.envoyMetadata)
	}
}

func WithEnvoyMetadataFile(file string) func(*OpenPolicyAgentInstanceConfig) error {
	return func(cfg *OpenPolicyAgentInstanceConfig) error {
		content, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		err = WithEnvoyMetadataBytes(content)(cfg)
		if err != nil {
			return fmt.Errorf("cannot parse '%q': %w", file, err)
		}

		return nil
	}
}

func (config *OpenPolicyAgentInstanceConfig) GetEnvoyMetadata() *ext_authz_v3_core.Metadata {
	if config.envoyMetadata != nil {
		return proto.Clone(config.envoyMetadata).(*ext_authz_v3_core.Metadata)
	}
	return nil
}

func NewOpenPolicyAgentConfig(opts ...func(*OpenPolicyAgentInstanceConfig) error) (*OpenPolicyAgentInstanceConfig, error) {
	cfg := OpenPolicyAgentInstanceConfig{}

	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	if cfg.configTemplate == nil {
		var err error
		cfg.configTemplate, err = os.ReadFile("opaconfig.yaml")
		if err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

func (registry *OpenPolicyAgentRegistry) Close() {
	registry.once.Do(func() {
		registry.mu.Lock()
		defer registry.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownGracePeriod)
		defer cancel()

		for _, instanceInfo := range registry.instances {
			if instanceInfo != nil && instanceInfo.instance != nil {
				instanceInfo.instance.Close(ctx)
				registry.singleflightGroup.Forget(instanceInfo.instance.bundleName)
			}
		}

		registry.closed = true
		close(registry.quit)

		// Close background task channel
		if registry.backgroundTaskChan != nil {
			close(registry.backgroundTaskChan)
		}
	})
}

// GetInstanceCount returns the number of instances in the registry (thread-safe)
func (registry *OpenPolicyAgentRegistry) GetInstanceCount() int {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return len(registry.instances)
}

// countInstancesByState returns the count of instances with the specified state
func (registry *OpenPolicyAgentRegistry) countInstancesByState(state InstanceState) int {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	count := 0
	for _, instanceInfo := range registry.instances {
		if instanceInfo.state == state {
			count++
		}
	}
	return count
}

// GetReadyInstanceCount returns the number of ready instances in the registry (thread-safe)
func (registry *OpenPolicyAgentRegistry) GetReadyInstanceCount() int {
	return registry.countInstancesByState(InstanceStateReady)
}

// GetFailedInstanceCount returns the number of failed instances in the registry (thread-safe)
func (registry *OpenPolicyAgentRegistry) GetFailedInstanceCount() int {
	return registry.countInstancesByState(InstanceStateFailed)
}

// GetLoadingInstanceCount returns the number of loading instances in the registry (thread-safe)
func (registry *OpenPolicyAgentRegistry) GetLoadingInstanceCount() int {
	return registry.countInstancesByState(InstanceStateLoading)
}

func (registry *OpenPolicyAgentRegistry) cleanUnusedInstances(t time.Time) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if registry.closed {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownGracePeriod)
	defer cancel()

	for key, instanceInfo := range registry.instances {
		if instanceInfo != nil && instanceInfo.instance != nil &&
			t.Sub(instanceInfo.lastUsed) > registry.reuseDuration {

			instanceInfo.instance.Close(ctx)
			delete(registry.instances, key)
			registry.singleflightGroup.Forget(key)
		}
	}
}

func (registry *OpenPolicyAgentRegistry) startCleanerDaemon() {
	ticker := time.NewTicker(registry.cleanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-registry.quit:
			return
		case t := <-ticker.C:
			registry.cleanUnusedInstances(t)
		}
	}
}

// startCustomControlLoopDaemon starts a custom control loop that triggers the discovery and bundle plugin for all OPA instances in the registry.
// The processing is done in sequence to avoid memory spikes if the bundles of multiple instances are updated at the same time.
// The timeout for the processing of each instance is set to the startup timeout to ensure that the behavior is the same as during startup
// It is accepted that runs can be skipped if the processing of all instances takes longer than the interval.
func (registry *OpenPolicyAgentRegistry) startCustomControlLoopDaemon() {
	ticker := time.NewTicker(registry.controlLoopIntervalWithJitter())
	defer ticker.Stop()

	for {
		select {
		case <-registry.quit:
			return
		case <-ticker.C:
			registry.mu.Lock()
			instances := slices.Collect(maps.Values(registry.instances))
			registry.mu.Unlock()

			for _, opaInfo := range instances {
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), registry.instanceStartupTimeout)
					defer cancel()
					opaInfo.instance.triggerPlugins(ctx)
				}()
			}
			ticker.Reset(registry.controlLoopIntervalWithJitter())
		}
	}
}

// Prevent different opa instances from triggering plugins (f.ex. downloading new bundles) at the same time
func (registry *OpenPolicyAgentRegistry) controlLoopIntervalWithJitter() time.Duration {
	if registry.controlLoopMaxJitter > 0 {
		return registry.controlLoopInterval + time.Duration(rand.Int63n(int64(registry.controlLoopMaxJitter))) - registry.controlLoopMaxJitter/2
	}
	return registry.controlLoopInterval
}

// Do implements routing.PostProcessor and cleans unused OPA instances
func (registry *OpenPolicyAgentRegistry) Do(routes []*routing.Route) []*routing.Route {
	inUse := make(map[*OpenPolicyAgentInstance]struct{})

	for _, ri := range routes {
		for _, fi := range ri.Filters {
			if ff, ok := fi.Filter.(OpenPolicyAgentFilter); ok {
				inUse[ff.OpenPolicyAgent()] = struct{}{}
			}
		}
	}

	registry.markUnused(inUse)

	return routes
}

// GetOrStartInstance returns an existing instance immediately, or creates one using registry config
func (registry *OpenPolicyAgentRegistry) GetOrStartInstance(bundleName string, filterName string) (*OpenPolicyAgentInstance, error) {
	// First check if instance already exists
	instance, err := registry.getExistingInstance(bundleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing OPA instance for bundle '%s': %w", bundleName, err)
	}

	if instance != nil {
		// Instance already exists, return it
		return instance, nil
	}

	if registry.preloadingEnabled {
		// In preloading mode, if instance doesn't exist, it means it's not ready yet
		return nil, fmt.Errorf("open policy agent instance for bundle '%s' is not ready yet", bundleName)
	}

	// In non-preloading mode, create the instance synchronously using PrepareInstanceLoader
	loader := registry.PrepareInstanceLoader(bundleName, filterName)
	return loader()
}

func (registry *OpenPolicyAgentRegistry) getExistingInstance(bundleName string) (*OpenPolicyAgentInstance, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if registry.closed {
		return nil, fmt.Errorf("open policy agent registry is already closed")
	}

	if instanceInfo, ok := registry.instances[bundleName]; ok {
		registry.instances[bundleName].lastUsed = time.Now()
		return instanceInfo.instance, nil
	}

	return nil, nil
}

// PrepareInstanceLoader returns a function that when called will create an OPA instance
// This allows the preprocessor to control when and how the instance creation happens
// Prevents concurrent creation of the same bundle by tracking in-flight operations
func (registry *OpenPolicyAgentRegistry) PrepareInstanceLoader(bundleName, filterName string) func() (*OpenPolicyAgentInstance, error) {
	return func() (*OpenPolicyAgentInstance, error) {
		// Fast path: already exists
		if inst, err := registry.getExistingInstance(bundleName); err != nil {
			return nil, fmt.Errorf("failed to get existing OPA instance for bundle %q: %w", bundleName, err)
		} else if inst != nil {
			return inst, nil
		}

		// Collapse concurrent creations into one using singleflight
		ch := registry.singleflightGroup.DoChan(bundleName, func() (any, error) {
			// Re-check after entering singleflight
			if inst, err := registry.getExistingInstance(bundleName); err != nil {
				registry.singleflightGroup.Forget(bundleName)
				return nil, fmt.Errorf("failed to recheck OPA instance for bundle %q: %w", bundleName, err)
			} else if inst != nil {
				return inst, nil
			}

			// Create new OPA instance
			inst, err := registry.newOpenPolicyAgentInstance(bundleName, filterName)
			if err != nil {
				registry.singleflightGroup.Forget(bundleName)
				registry.setInstanceFailed(bundleName, err)
				return nil, err
			}

			// Cache instance
			registry.setInstanceReady(bundleName, inst)

			return inst, nil
		})

		// Coordination timeout: longer than the plugin startup timeout to allow detailed error propagation
		// This protects against singleflight goroutine death/hang, apart from HTTP timeouts
		coordinationTimeout := 3 * registry.instanceStartupTimeout // 3x longer - this implies in rare cases startup can take up to 3 times configured timeout
		coordinationTimer := time.NewTimer(coordinationTimeout)
		defer coordinationTimer.Stop()

		select {
		case res := <-ch:
			if res.Err != nil {
				return nil, res.Err // Detailed HTTP error (429, 404, 500, etc.)
			}
			return res.Val.(*OpenPolicyAgentInstance), nil
		case <-coordinationTimer.C:
			// This should rarely/never fire - only for catastrophic failures
			registry.singleflightGroup.Forget(bundleName)
			return nil, fmt.Errorf("coordination timeout: singleflight goroutine appears to have failed for bundle %q", bundleName)
		}
	}
}

func (registry *OpenPolicyAgentRegistry) setInstanceState(bundleName string, state InstanceState, inst *OpenPolicyAgentInstance, err error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	var lastUsed time.Time
	if state == InstanceStateReady {
		lastUsed = time.Now()
	}

	registry.instances[bundleName] = &InstanceInfo{
		instance: inst,
		state:    state,
		lastUsed: lastUsed,
		error:    err,
	}
}

func (registry *OpenPolicyAgentRegistry) setInstanceReady(bundleName string, inst *OpenPolicyAgentInstance) {
	registry.setInstanceState(bundleName, InstanceStateReady, inst, nil)
}

func (registry *OpenPolicyAgentRegistry) setInstanceFailed(bundleName string, err error) {
	registry.setInstanceState(bundleName, InstanceStateFailed, nil, err)
}

func (registry *OpenPolicyAgentRegistry) setInstanceLoading(bundleName string) {
	registry.setInstanceState(bundleName, InstanceStateLoading, nil, nil)
}

func (registry *OpenPolicyAgentRegistry) markUnused(inUse map[*OpenPolicyAgentInstance]struct{}) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	for _, instanceInfo := range registry.instances {
		if instanceInfo != nil && instanceInfo.instance != nil {
			if _, ok := inUse[instanceInfo.instance]; !ok {
				instanceInfo.lastUsed = time.Now()
			}
		}
	}
}

func (registry *OpenPolicyAgentRegistry) newOpenPolicyAgentInstance(bundleName string, filterName string) (*OpenPolicyAgentInstance, error) {
	runtime.RegisterPlugin(envoy.PluginName, envoy.Factory{})

	engine, err := registry.new(inmem.NewWithOpts(inmem.OptReturnASTValuesOnRead(registry.enableDataPreProcessingOptimization)), filterName, bundleName,
		registry.maxRequestBodyBytes, registry.bodyReadBufferSize)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), registry.instanceStartupTimeout)
	defer cancel()

	if registry.enableCustomControlLoop {
		if err = engine.StartAndTriggerPlugins(ctx); err != nil {
			return nil, err
		}
	} else {
		if err = engine.Start(ctx, registry.instanceStartupTimeout); err != nil {
			return nil, err
		}
	}

	return engine, nil
}

type OpenPolicyAgentInstance struct {
	manager                     *plugins.Manager
	instanceConfig              OpenPolicyAgentInstanceConfig
	opaConfig                   *config.Config
	bundleName                  string
	preparedQuery               *rego.PreparedEvalQuery
	preparedQueryDoOnce         *sync.Once
	preparedQueryErr            error
	interQueryBuiltinCache      iCache.InterQueryCache
	interQueryBuiltinValueCache iCache.InterQueryValueCache
	once                        sync.Once
	closing                     bool
	registry                    *OpenPolicyAgentRegistry

	maxBodyBytes       int64
	bodyReadBufferSize int64

	idGenerator flowid.Generator
}

func envVariablesMap() map[string]string {
	rawEnvVariables := os.Environ()
	envVariables := make(map[string]string)

	for _, item := range rawEnvVariables {
		tokens := strings.SplitN(item, "=", 2)
		envVariables[tokens[0]] = tokens[1]
	}

	return envVariables
}

// Config sets the configuration file to use on the OPA instance.
func (config *OpenPolicyAgentInstanceConfig) interpolateConfigTemplate(bundleName string) ([]byte, error) {
	var buf bytes.Buffer

	tpl := template.Must(template.New("opa-config").Parse(string(config.configTemplate)))

	binding := make(map[string]interface{})
	binding["bundlename"] = bundleName
	binding["Env"] = envVariablesMap()

	err := tpl.ExecuteTemplate(&buf, "opa-config", binding)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func buildTracingOptions(tracer opentracing.Tracer, bundleName string, manager *plugins.Manager) opatracing.Options {
	return opatracing.NewOptions(WithTracingOptTracer(tracer), WithTracingOptBundleName(bundleName), WithTracingOptManager(manager))
}

func (registry *OpenPolicyAgentRegistry) withTracingOptions(bundleName string) func(*plugins.Manager) {
	return func(m *plugins.Manager) {
		options := buildTracingOptions(
			registry.tracer,
			bundleName,
			m,
		)

		plugins.WithDistributedTracingOpts(options)(m)
	}
}

// new returns a new OPA object.
func (registry *OpenPolicyAgentRegistry) new(store storage.Store, filterName string, bundleName string, maxBodyBytes int64, bodyReadBufferSize int64) (*OpenPolicyAgentInstance, error) {
	id := uuid.New().String()
	uniqueIDGenerator, err := flowid.NewStandardGenerator(32)
	if err != nil {
		return nil, err
	}

	configBytes, err := registry.configTemplate.interpolateConfigTemplate(bundleName)
	if err != nil {
		return nil, err
	}

	opaConfig, err := config.ParseConfig(configBytes, id)
	if err != nil {
		return nil, err
	}

	runtime.RegisterPlugin(envoy.PluginName, envoy.Factory{})

	var logger logging.Logger = &QuietLogger{target: logging.Get()}
	logger = logger.WithFields(map[string]interface{}{"skipper-filter": filterName, "bundle-name": bundleName})

	configHooks := hooks.New()
	if registry.enableCustomControlLoop {
		configHooks = hooks.New(&internal.ManualOverride{})
	}

	manager, err := plugins.New(configBytes, id, store, configLabelsInfo(*opaConfig), plugins.Logger(logger), registry.withTracingOptions(bundleName), plugins.WithHooks(configHooks))

	if err != nil {
		return nil, err
	}

	discovery, err := discovery.New(manager, discovery.Factories(map[string]plugins.Factory{envoy.PluginName: envoy.Factory{}}), discovery.Hooks(configHooks))
	if err != nil {
		return nil, err
	}

	manager.Register("discovery", discovery)

	opa := &OpenPolicyAgentInstance{
		registry:       registry,
		instanceConfig: *registry.configTemplate,
		manager:        manager,
		opaConfig:      opaConfig,
		bundleName:     bundleName,

		maxBodyBytes:       maxBodyBytes,
		bodyReadBufferSize: bodyReadBufferSize,

		preparedQueryDoOnce:         new(sync.Once),
		interQueryBuiltinCache:      iCache.NewInterQueryCache(manager.InterQueryBuiltinCacheConfig()),
		interQueryBuiltinValueCache: registry.valueCache,

		idGenerator: uniqueIDGenerator,
	}

	manager.RegisterCompilerTrigger(opa.compilerUpdated)

	return opa, nil
}

// Start asynchronously starts the policy engine's plugins that download
// policies, report status, etc.
func (opa *OpenPolicyAgentInstance) Start(ctx context.Context, timeout time.Duration) error {
	err := opa.manager.Start(ctx)

	if err != nil {
		return err
	}

	// check readiness of all plugins
	pluginsReady := func() bool {
		for _, status := range opa.manager.PluginStatus() {
			if status != nil && status.State != plugins.StateOK {
				return false
			}
		}
		return true
	}

	err = waitFunc(ctx, pluginsReady, 100*time.Millisecond)

	if err != nil {
		for pluginName, status := range opa.manager.PluginStatus() {
			if status != nil && status.State != plugins.StateOK {
				opa.Logger().WithFields(map[string]interface{}{
					"plugin_name":   pluginName,
					"plugin_state":  status.State,
					"error_message": status.Message,
				}).Error("Open policy agent plugin did not start in time")
			}
		}
		opa.Close(ctx)
		return fmt.Errorf("one or more open policy agent plugins failed to start in %v with error: %w", timeout, err)
	}
	return nil
}

// StartAndTriggerPlugins Start starts the policy engine's plugin manager and triggers the plugins to download policies etc.
func (opa *OpenPolicyAgentInstance) StartAndTriggerPlugins(ctx context.Context) error {
	err := opa.manager.Start(ctx)
	if err != nil {
		return err
	}

	err = opa.triggerPluginsWithRetry(ctx)
	if err != nil {
		opa.Close(ctx)
		return err
	}

	err = opa.verifyAllPluginsStarted()
	if err != nil {
		opa.Close(ctx)
		return err
	}
	return nil
}

func (opa *OpenPolicyAgentInstance) triggerPluginsWithRetry(ctx context.Context) error {
	var err error
	backoff := 100 * time.Millisecond
	retryTrigger := time.NewTimer(backoff)
	defer retryTrigger.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while triggering plugins: %w, last retry returned: %w", ctx.Err(), err)
		case <-retryTrigger.C:
			err = opa.triggerPlugins(ctx)

			if !opa.isRetryable(err) {
				return err
			}
			backoff *= 2
			retryTrigger.Reset(backoff)
		}
	}
}

func (opa *OpenPolicyAgentInstance) isRetryable(err error) bool {
	var httpError download.HTTPError

	if errors.As(err, &httpError) {
		opa.Logger().WithFields(map[string]interface{}{
			"error": httpError.Error(),
		}).Warn("Triggering bundles failed. Response code %v, Retrying.", httpError.StatusCode)
		return httpError.StatusCode == 429 || httpError.StatusCode >= 500
	}

	var urlError *url.Error
	if errors.As(err, &urlError) {
		retry := strings.Contains(urlError.Error(), "net/http: timeout awaiting response headers")
		if retry {
			opa.Logger().WithFields(map[string]interface{}{
				"error": urlError.Error(),
			}).Warn("Triggering bundles failed. Retrying.")
		}
		return retry
	}
	return false
}

func (opa *OpenPolicyAgentInstance) verifyAllPluginsStarted() error {
	allPluginsReady := true
	for pluginName, status := range opa.manager.PluginStatus() {
		if status != nil && status.State != plugins.StateOK {
			opa.Logger().WithFields(map[string]interface{}{
				"plugin_name":   pluginName,
				"plugin_state":  status.State,
				"error_message": status.Message,
			}).Error("Open policy agent plugin failed to start %s", pluginName)

			allPluginsReady = false
		}
	}
	if !allPluginsReady {
		return fmt.Errorf("open policy agent plugins failed to start")
	}
	return nil
}

func (opa *OpenPolicyAgentInstance) triggerPlugins(ctx context.Context) error {

	if opa.closing {
		return nil
	}
	for _, pluginName := range []string{"discovery", "bundle"} {
		if plugin, ok := opa.manager.Plugin(pluginName).(plugins.Triggerable); ok {
			if err := plugin.Trigger(ctx); err != nil {
				return err
			}

		} else if pluginName == "bundle" { // only fail for bundle plugin as discovery plugin is optional
			return fmt.Errorf("plugin %s not found", pluginName)
		}
	}
	return nil
}

func (opa *OpenPolicyAgentInstance) Close(ctx context.Context) {
	opa.once.Do(func() {
		opa.closing = true
		opa.manager.Stop(ctx)
	})
}

func waitFunc(ctx context.Context, fun func() bool, interval time.Duration) error {
	if fun() {
		return nil
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out while starting: %w", ctx.Err())
		case <-ticker.C:
			if fun() {
				return nil
			}
		}
	}
}

func configLabelsInfo(opaConfig config.Config) func(*plugins.Manager) {
	info := ast.NewObject()
	labels := ast.NewObject()
	labelsWrapper := ast.NewObject()

	for key, value := range opaConfig.Labels {
		labels.Insert(ast.StringTerm(key), ast.StringTerm(value))
	}

	labelsWrapper.Insert(ast.StringTerm("labels"), ast.NewTerm(labels))
	info.Insert(ast.StringTerm("config"), ast.NewTerm(labelsWrapper))

	return plugins.Info(ast.NewTerm(info))
}

func (opa *OpenPolicyAgentInstance) InstanceConfig() *OpenPolicyAgentInstanceConfig {
	return &opa.instanceConfig
}

func (opa *OpenPolicyAgentInstance) compilerUpdated(txn storage.Transaction) {
	opa.preparedQueryDoOnce = new(sync.Once)
}

func (opa *OpenPolicyAgentInstance) EnvoyPluginConfig() envoy.PluginConfig {
	if plugin, ok := opa.manager.Plugin(envoy.PluginName).(*envoy.Plugin); ok {
		return plugin.GetConfig()
	}

	defaultConfig := envoy.PluginConfig{
		Path:   "envoy/authz/allow",
		DryRun: false,
	}
	defaultConfig.ParseQuery()
	return defaultConfig
}

func setSpanTags(span opentracing.Span, bundleName string, manager *plugins.Manager) {
	if bundleName != "" {
		span.SetTag("opa.bundle_name", bundleName)
	}

	if manager != nil {
		for label, value := range manager.Labels() {
			span.SetTag("opa.label."+label, value)
		}
	}
}

func (opa *OpenPolicyAgentInstance) startSpanFromContextWithTracer(tr opentracing.Tracer, parent opentracing.Span, ctx context.Context) (opentracing.Span, context.Context) {

	var span opentracing.Span
	if parent != nil {
		span = tr.StartSpan(spanNameEval, opentracing.ChildOf(parent.Context()))
	} else {
		span = tracing.CreateSpan(spanNameEval, ctx, tr)
	}

	setSpanTags(span, opa.bundleName, opa.manager)

	return span, opentracing.ContextWithSpan(ctx, span)
}

func (opa *OpenPolicyAgentInstance) StartSpanFromFilterContext(fc filters.FilterContext) (opentracing.Span, context.Context) {
	return opa.StartSpanFromContext(fc.Request().Context())
}

func (opa *OpenPolicyAgentInstance) StartSpanFromContext(ctx context.Context) (opentracing.Span, context.Context) {
	span := opentracing.SpanFromContext(ctx)
	if span != nil {
		return opa.startSpanFromContextWithTracer(span.Tracer(), span, ctx)
	}

	return opa.startSpanFromContextWithTracer(opentracing.GlobalTracer(), nil, ctx)
}

func (opa *OpenPolicyAgentInstance) MetricsKey(key string) string {
	return key + "." + opa.bundleName
}

var (
	ErrClosed                 = errors.New("reader closed")
	ErrTotalBodyBytesExceeded = errors.New("buffer for in-flight request body authorization in Open Policy Agent exceeded")
)

type bufferedBodyReader struct {
	input         io.ReadCloser
	maxBufferSize int64

	bodyBuffer bytes.Buffer
	readBuffer []byte

	once   sync.Once
	err    error
	closed bool
}

func newBufferedBodyReader(input io.ReadCloser, maxBufferSize int64, readBufferSize int64) *bufferedBodyReader {
	return &bufferedBodyReader{
		input:         input,
		maxBufferSize: maxBufferSize,
		readBuffer:    make([]byte, readBufferSize),
	}
}

func (m *bufferedBodyReader) fillBuffer(expectedSize int64) ([]byte, error) {
	var err error

	for err == nil && int64(m.bodyBuffer.Len()) < m.maxBufferSize && int64(m.bodyBuffer.Len()) < expectedSize {
		var n int
		n, err = m.input.Read(m.readBuffer)

		m.bodyBuffer.Write(m.readBuffer[:n])
	}

	if err == io.EOF {
		err = nil
	}

	return m.bodyBuffer.Bytes(), err
}

func (m *bufferedBodyReader) Read(p []byte) (int, error) {
	if m.closed {
		return 0, ErrClosed
	}

	if m.err != nil {
		return 0, m.err
	}

	// First read the buffered body
	if m.bodyBuffer.Len() != 0 {
		return m.bodyBuffer.Read(p)
	}

	// Continue reading from the underlying body reader
	return m.input.Read(p)
}

// Close closes the undelrying reader if it implements io.Closer.
func (m *bufferedBodyReader) Close() error {
	var err error
	m.once.Do(func() {
		m.closed = true
		if c, ok := m.input.(io.Closer); ok {
			err = c.Close()
		}
	})
	return err
}

func bodyUpperBound(contentLength, maxBodyBytes int64) int64 {
	if contentLength <= 0 {
		return maxBodyBytes
	}

	if contentLength < maxBodyBytes {
		return contentLength
	}

	return maxBodyBytes
}

func (opa *OpenPolicyAgentInstance) ExtractHttpBodyOptionally(req *http.Request) (io.ReadCloser, []byte, func(), error) {
	body := req.Body

	if body != nil && !opa.EnvoyPluginConfig().SkipRequestBodyParse &&
		req.ContentLength <= int64(opa.maxBodyBytes) {

		wrapper := newBufferedBodyReader(req.Body, opa.maxBodyBytes, opa.bodyReadBufferSize)

		requestedBodyBytes := bodyUpperBound(req.ContentLength, opa.maxBodyBytes)
		if !opa.registry.maxMemoryBodyParsingSem.TryAcquire(requestedBodyBytes) {
			return req.Body, nil, func() {}, ErrTotalBodyBytesExceeded
		}

		rawBody, err := wrapper.fillBuffer(req.ContentLength)
		return wrapper, rawBody, func() { opa.registry.maxMemoryBodyParsingSem.Release(requestedBodyBytes) }, err
	}

	return req.Body, nil, func() {}, nil
}

// ParsedQuery is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) ParsedQuery() ast.Body {
	return opa.EnvoyPluginConfig().ParsedQuery
}

// Store is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) Store() storage.Store { return opa.manager.Store }

// Compiler is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) Compiler() *ast.Compiler { return opa.manager.GetCompiler() }

// Runtime is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) Runtime() *ast.Term { return opa.manager.Info }

// Logger is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) Logger() logging.Logger { return opa.manager.Logger() }

// InterQueryBuiltinCache is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) InterQueryBuiltinCache() iCache.InterQueryCache {
	return opa.interQueryBuiltinCache
}

// InterQueryBuiltinValueCache is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) InterQueryBuiltinValueCache() iCache.InterQueryValueCache {
	return opa.interQueryBuiltinValueCache
}

// Config is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) Config() *config.Config { return opa.opaConfig }

// DistributedTracing is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) DistributedTracing() opatracing.Options {
	return buildTracingOptions(opa.registry.tracer, opa.bundleName, opa.manager)
}

// CreatePreparedQueryOnce is an implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) CreatePreparedQueryOnce(opts envoyauth.PrepareQueryOpts) (*rego.PreparedEvalQuery, error) {
	opa.preparedQueryDoOnce.Do(func() {
		regoOpts := append(opts.Opts, rego.DistributedTracingOpts(opa.DistributedTracing()))

		pq, err := rego.New(regoOpts...).PrepareForEval(context.Background())

		opa.preparedQuery = &pq
		opa.preparedQueryErr = err
	})

	return opa.preparedQuery, opa.preparedQueryErr
}

// logging.Logger that does not pollute info with debug logs
type QuietLogger struct {
	target logging.Logger
}

func (l *QuietLogger) WithFields(fields map[string]interface{}) logging.Logger {
	return &QuietLogger{target: l.target.WithFields(fields)}
}

func (l *QuietLogger) SetLevel(level logging.Level) {
	l.target.SetLevel(level)
}

func (l *QuietLogger) GetLevel() logging.Level {
	return l.target.GetLevel()
}

func (l *QuietLogger) Debug(fmt string, a ...interface{}) {
	l.target.Debug(fmt, a)
}

func (l *QuietLogger) Info(fmt string, a ...interface{}) {
	l.target.Debug(fmt, a)
}

func (l *QuietLogger) Error(fmt string, a ...interface{}) {
	l.target.Error(fmt, a)
}

func (l *QuietLogger) Warn(fmt string, a ...interface{}) {
	l.target.Warn(fmt, a)
}

// ScheduleBackgroundTask schedules a task to be executed in the background with limited parallelism (1)
// Returns a BackgroundTask that can be used to wait for completion
func (registry *OpenPolicyAgentRegistry) ScheduleBackgroundTask(fn func() (interface{}, error)) (*BackgroundTask, error) {
	task := &BackgroundTask{
		fn:   fn,
		done: make(chan struct{}),
	}

	// Start the background worker if not already started
	registry.startBackgroundWorker()

	// Send the task to the worker
	select {
	case registry.backgroundTaskChan <- task:
		return task, nil
	default:
		return nil, fmt.Errorf("open policy agent background task queue is full, try again later")
	}
}

// startBackgroundWorker starts the background worker goroutine (thread-safe, only starts once)
func (registry *OpenPolicyAgentRegistry) startBackgroundWorker() {
	registry.backgroundWorkerOnce.Do(func() {
		go func() {
			for task := range registry.backgroundTaskChan {
				if task != nil {
					task.execute()
				}
			}
		}()
	})
}
