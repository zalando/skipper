package openpolicyagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	dl "github.com/open-policy-agent/eopa/pkg/plugins/decision_logs"
	"github.com/open-policy-agent/opa/v1/util"

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
	"github.com/open-policy-agent/opa/v1/plugins/bundle"
	"github.com/open-policy-agent/opa/v1/plugins/discovery"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/runtime"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	iCache "github.com/open-policy-agent/opa/v1/topdown/cache"
	opatracing "github.com/open-policy-agent/opa/v1/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
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
	DefaultCleanIdlePeriod          = 10 * time.Second
	DefaultControlLoopInterval      = 60 * time.Second
	DefaultControlLoopMaxJitter     = 1000 * time.Millisecond
	defaultReuseDuration            = 30 * time.Second
	defaultShutdownGracePeriod      = 30 * time.Second
	DefaultOpaStartupTimeout        = 30 * time.Second
	DefaultBackgroundTaskBufferSize = 100

	DefaultMaxRequestBodySize    = 1 << 20 // 1 MB
	DefaultMaxMemoryBodyParsing  = 100 * DefaultMaxRequestBodySize
	DefaultRequestBodyBufferSize = 8 * 1024 // 8 KB

	spanNameEval = "open-policy-agent"
)

type BackgroundTask struct {
	fn   func() error
	done chan struct{}
	err  error
	once sync.Once
}

// Wait blocks until the task completes and returns the result and error
func (t *BackgroundTask) Wait() error {
	<-t.done
	return t.err
}

// execute runs the task function and stores the result
func (t *BackgroundTask) execute() {
	t.once.Do(func() {
		defer close(t.done)
		t.err = t.fn()
	})
}

type OpenPolicyAgentRegistry struct {
	// Ideally share one Bundle storage across many OPA "instances" using this registry.
	// This allows to save memory on bundles that are shared
	// between different policies (i.e. global team memberships)
	// This not possible due to some limitations in OPA
	// See https://github.com/open-policy-agent/opa/issues/5707

	mu        sync.Mutex
	instances map[string]*OpenPolicyAgentInstance
	lastused  map[*OpenPolicyAgentInstance]time.Time

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

	tracer               opentracing.Tracer
	prometheusRegisterer prometheus.Registerer

	enableCustomControlLoop bool
	controlLoopInterval     time.Duration
	controlLoopMaxJitter    time.Duration

	enableDataPreProcessingOptimization bool

	valueCache iCache.InterQueryValueCache

	// New fields for pre-loading support
	preloadingEnabled bool

	// Background task system
	backgroundTaskChan       chan *BackgroundTask
	backgroundWorkerOnce     sync.Once
	backgroundTaskBufferSize int
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

func WithPrometheusRegisterer(registerer prometheus.Registerer) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.prometheusRegisterer = registerer
		return nil
	}
}

func (registry *OpenPolicyAgentRegistry) initializeCache() error {
	// This line interpolates the config template with a dummy bundle name to make sure the config is parseable.
	// It is safe in production because the result is not used for anything except caching configuration.
	configBytes, err := registry.configTemplate.interpolateConfigTemplate("dummy-bundle-name")
	if err != nil {
		return fmt.Errorf("failed to interpolate opa config template: %w", err)
	}

	id := uuid.New().String()
	parsedConfig, err := config.ParseConfig(configBytes, id)
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

func WithBackgroundTaskBufferSize(size int) func(*OpenPolicyAgentRegistry) error {
	return func(cfg *OpenPolicyAgentRegistry) error {
		cfg.backgroundTaskBufferSize = size
		return nil
	}
}

func NewOpenPolicyAgentRegistry(opts ...func(*OpenPolicyAgentRegistry) error) (*OpenPolicyAgentRegistry, error) {
	registry := &OpenPolicyAgentRegistry{
		reuseDuration:            defaultReuseDuration,
		cleanInterval:            DefaultCleanIdlePeriod,
		instanceStartupTimeout:   DefaultOpaStartupTimeout,
		instances:                make(map[string]*OpenPolicyAgentInstance),
		lastused:                 make(map[*OpenPolicyAgentInstance]time.Time),
		quit:                     make(chan struct{}),
		maxRequestBodyBytes:      DefaultMaxMemoryBodyParsing,
		bodyReadBufferSize:       DefaultRequestBodyBufferSize,
		controlLoopInterval:      DefaultControlLoopInterval,
		controlLoopMaxJitter:     DefaultControlLoopMaxJitter,
		backgroundTaskBufferSize: DefaultBackgroundTaskBufferSize,
	}

	for _, opt := range opts {
		opt(registry)
	}
	registry.backgroundTaskChan = make(chan *BackgroundTask, registry.backgroundTaskBufferSize)

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

		for _, instance := range registry.instances {
			instance.Close(ctx)
		}

		registry.closed = true
		close(registry.quit)

		// Close background task channel
		if registry.backgroundTaskChan != nil {
			close(registry.backgroundTaskChan)
		}
	})
}

func (registry *OpenPolicyAgentRegistry) cleanUnusedInstances(t time.Time) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if registry.closed {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownGracePeriod)
	defer cancel()

	for key, inst := range registry.instances {
		lastused, ok := registry.lastused[inst]

		if ok && t.Sub(lastused) > registry.reuseDuration {
			inst.Close(ctx)

			delete(registry.instances, key)
			delete(registry.lastused, inst)
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

			for _, opa := range instances {
				if opa != nil && !opa.Started() { // Skip retriggering plugins of unstarted instances
					continue
				}
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), registry.instanceStartupTimeout)
					defer cancel()
					opa.triggerPlugins(ctx)
				}()
			}
			ticker.Reset(registry.controlLoopIntervalWithJitter())
		}
	}
}

// Prevent different opa instances from triggering plugins (f.ex. downloading new bundles) at the same time
func (registry *OpenPolicyAgentRegistry) controlLoopIntervalWithJitter() time.Duration {
	if registry.controlLoopMaxJitter > 0 {
		return registry.controlLoopInterval + time.Duration(rand.Int64N(int64(registry.controlLoopMaxJitter))) - registry.controlLoopMaxJitter/2 // #nosec
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
func (registry *OpenPolicyAgentRegistry) GetOrStartInstance(bundleName string) (*OpenPolicyAgentInstance, error) {
	// First check if instance already exists
	instance, err := registry.getExistingInstance(bundleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing OPA instance for bundle %q : %w", bundleName, err)
	}

	if instance != nil {
		// Instance already exists, return it
		return instance, nil
	}

	if registry.preloadingEnabled {
		// In preloading mode, if instance doesn't exist, it means a fatal error occurred while creating the opa instance
		return nil, fmt.Errorf("open policy agent instance for bundle %q could not be created", bundleName)
	}

	// In non-preloading mode, create and start the instance synchronously
	inst, err := registry.createAndCacheInstance(bundleName)
	if err != nil {
		return nil, fmt.Errorf("failed to create OPA instance for bundle %q: %w", bundleName, err)
	}
	err = inst.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start OPA instance for bundle %q: %w", bundleName, err)
	}

	return inst, nil
}

func (registry *OpenPolicyAgentRegistry) getExistingInstance(bundleName string) (*OpenPolicyAgentInstance, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if registry.closed {
		return nil, fmt.Errorf("open policy agent registry is already closed")
	}

	if instance, ok := registry.instances[bundleName]; ok {
		delete(registry.lastused, instance)
		return instance, nil
	}

	return nil, nil
}

func (registry *OpenPolicyAgentRegistry) createAndCacheInstance(bundleName string) (*OpenPolicyAgentInstance, error) {
	// Create new OPA instance
	inst, err := registry.newOpenPolicyAgentInstance(bundleName)
	if err != nil {
		return nil, err
	}

	// Cache instance
	registry.mu.Lock()
	registry.instances[bundleName] = inst
	registry.mu.Unlock()

	return inst, nil
}

func (registry *OpenPolicyAgentRegistry) markUnused(inUse map[*OpenPolicyAgentInstance]struct{}) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	for _, instance := range registry.instances {
		if _, ok := inUse[instance]; !ok {
			registry.lastused[instance] = time.Now()
		}
	}
}

func (registry *OpenPolicyAgentRegistry) newOpenPolicyAgentInstance(bundleName string) (*OpenPolicyAgentInstance, error) {
	runtime.RegisterPlugin(envoy.PluginName, envoy.Factory{})

	engine, err := registry.new(inmem.NewWithOpts(inmem.OptReturnASTValuesOnRead(registry.enableDataPreProcessingOptimization)), bundleName,
		registry.maxRequestBodyBytes, registry.bodyReadBufferSize)
	if err != nil {
		return nil, err
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
	closingOnce                 sync.Once
	closing                     bool
	startedOnce                 sync.Once
	startingErr                 error
	started                     atomic.Bool
	startScheduled              atomic.Bool
	registry                    *OpenPolicyAgentRegistry
	healthy                     atomic.Bool
	logger                      logging.Logger

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
func (registry *OpenPolicyAgentRegistry) new(store storage.Store, bundleName string, maxBodyBytes int64, bodyReadBufferSize int64) (*OpenPolicyAgentInstance, error) {
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
	logger = logger.WithFields(map[string]interface{}{"bundle-name": bundleName})

	var configHooks []hooks.Hook
	if registry.enableCustomControlLoop {
		configHooks = append(configHooks, &internal.ManualOverride{})
	}

	var registerer prometheus.Registerer
	if registry.prometheusRegisterer != nil {
		registerer = prometheus.WrapRegistererWith(
			prometheus.Labels{
				"opa_instance_name": bundleName,
				"opa_instance_id":   id,
			},
			registry.prometheusRegisterer,
		)

		configHooks = append(configHooks, &internal.PrometheusOverride{})
	}

	manager, err := plugins.New(configBytes,
		id,
		store,
		configLabelsInfo(*opaConfig),
		plugins.Logger(logger),
		registry.withTracingOptions(bundleName),
		plugins.WithHooks(hooks.New(configHooks...)),
		plugins.WithPrometheusRegister(registerer))

	if err != nil {
		return nil, err
	}

	pluginFactories := map[string]plugins.Factory{envoy.PluginName: envoy.Factory{}, dl.DLPluginName: dl.Factory()}

	var bootConfig map[string]any
	err = util.Unmarshal(configBytes, &bootConfig)
	if err != nil {
		return nil, err
	}

	discoveryPlugin, err := discovery.New(manager, discovery.Factories(pluginFactories), discovery.Hooks(configHooks), discovery.BootConfig(bootConfig))
	if err != nil {
		return nil, err
	}

	manager.Register("discovery", discoveryPlugin)

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
		logger:      logging.Get().WithFields(map[string]any{"bundle-name": bundleName}),
	}

	manager.RegisterCompilerTrigger(opa.compilerUpdated)
	manager.RegisterPluginStatusListener("instance-health-check", func(_ map[string]*plugins.Status) {
		// Get fresh status to workaround OPA issue https://github.com/open-policy-agent/opa/issues/8009
		status := opa.manager.PluginStatus()
		opa.healthy.Store(allPluginsReady(status, bundle.Name, discovery.Name))
		opa.Logger().Info("OPA instance health updated: healthy=%t status=%+v", opa.healthy.Load(), status)
	})

	return opa, nil
}

func allPluginsReady(allPluginsStatus map[string]*plugins.Status, pluginNames ...string) bool {
	for pluginName, status := range allPluginsStatus {
		if pluginNames != nil && !slices.Contains(pluginNames, pluginName) {
			continue
		}

		if status != nil && status.State != plugins.StateOK {
			return false
		}
	}
	return true
}

func (opa *OpenPolicyAgentInstance) Start() error {
	opa.Logger().Info("Starting OPA instance...")
	opa.startedOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), opa.registry.instanceStartupTimeout)
		defer cancel()

		if opa.registry.enableCustomControlLoop {
			opa.Logger().Info("Custom control loop enabled, starting and triggering plugins")
			opa.startingErr = opa.startAndTriggerPlugins(ctx)
		} else {
			opa.startingErr = opa.start(ctx, opa.registry.instanceStartupTimeout)
		}

		opa.started.Store(true)
	})
	return opa.startingErr
}

// Start asynchronously starts the policy engine's plugins that download
// policies, report status, etc.
func (opa *OpenPolicyAgentInstance) start(ctx context.Context, timeout time.Duration) error {
	err := opa.manager.Start(ctx)

	if err != nil {
		return err
	}

	// check readiness of all plugins
	pluginsReady := func() bool {
		return allPluginsReady(opa.manager.PluginStatus())
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
		if !opa.registry.preloadingEnabled {
			opa.Close(ctx)
		}
		return fmt.Errorf("one or more open policy agent plugins failed to start in %v with error: %w", timeout, err)
	}
	return nil
}

func (opa *OpenPolicyAgentInstance) Healthy() bool {
	if opa == nil {
		return false
	}
	return opa.healthy.Load()
}

func (opa *OpenPolicyAgentInstance) Started() bool {
	return opa.started.Load()
}

func (opa *OpenPolicyAgentInstance) StartScheduled() bool {
	return opa.startScheduled.Load()
}

func (opa *OpenPolicyAgentInstance) MarkStartScheduled() {
	opa.startScheduled.Store(true)
}

// StartAndTriggerPlugins Start starts the policy engine's plugin manager and triggers the plugins to download policies etc.
func (opa *OpenPolicyAgentInstance) startAndTriggerPlugins(ctx context.Context) error {
	err := opa.manager.Start(ctx)
	if err != nil {
		return err
	}

	err = opa.triggerPluginsWithRetry(ctx)
	if err != nil {
		if !opa.registry.preloadingEnabled {
			opa.Close(ctx)
		}
		return err
	}

	err = opa.verifyAllPluginsStarted()
	if err != nil {
		if !opa.registry.preloadingEnabled {
			opa.Close(ctx)
		}
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
	opa.closingOnce.Do(func() {
		opa.Logger().Info("Closing OPA instance...")
		opa.closing = true
		opa.manager.Stop(ctx)
	})
}

func (opa *OpenPolicyAgentInstance) Logger() logging.Logger {
	if opa.logger != nil {
		return opa.logger
	}
	return nil
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

// Close closes the underlying reader if it implements io.Closer.
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

type evalContext struct {
	opa *OpenPolicyAgentInstance
}

// ParsedQuery is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) ParsedQuery() ast.Body {
	return ec.opa.EnvoyPluginConfig().ParsedQuery
}

// Store is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) Store() storage.Store { return ec.opa.manager.Store }

// Compiler is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) Compiler() *ast.Compiler { return ec.opa.manager.GetCompiler() }

// Runtime is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) Runtime() *ast.Term { return ec.opa.manager.Info }

// Logger is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) Logger() logging.Logger { return ec.opa.manager.Logger() }

// InterQueryBuiltinCache is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) InterQueryBuiltinCache() iCache.InterQueryCache {
	return ec.opa.interQueryBuiltinCache
}

// InterQueryBuiltinValueCache is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) InterQueryBuiltinValueCache() iCache.InterQueryValueCache {
	return ec.opa.interQueryBuiltinValueCache
}

// Config is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) Config() *config.Config { return ec.opa.opaConfig }

// DistributedTracing is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) DistributedTracing() opatracing.Options {
	return buildTracingOptions(ec.opa.registry.tracer, ec.opa.bundleName, ec.opa.manager)
}

// CreatePreparedQueryOnce is an implementation of the envoyauth.EvalContext interface
func (ec *evalContext) CreatePreparedQueryOnce(opts envoyauth.PrepareQueryOpts) (*rego.PreparedEvalQuery, error) {
	ec.opa.preparedQueryDoOnce.Do(func() {
		regoOpts := append(opts.Opts, rego.DistributedTracingOpts(ec.DistributedTracing()))

		pq, err := rego.New(regoOpts...).PrepareForEval(context.Background())

		ec.opa.preparedQuery = &pq
		ec.opa.preparedQueryErr = err
	})

	return ec.opa.preparedQuery, ec.opa.preparedQueryErr
}

// QuietLogger is a logging.Logger that does not pollute info with debug logs
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
	l.target.Debug(fmt, a...)
}

func (l *QuietLogger) Info(fmt string, a ...interface{}) {
	l.target.Debug(fmt, a...)
}

func (l *QuietLogger) Error(fmt string, a ...interface{}) {
	l.target.Error(fmt, a...)
}

func (l *QuietLogger) Warn(fmt string, a ...interface{}) {
	l.target.Warn(fmt, a...)
}

// ScheduleBackgroundTask schedules a task to be executed in the background with limited parallelism (1)
// Returns a BackgroundTask that can be used to wait for completion
func (registry *OpenPolicyAgentRegistry) ScheduleBackgroundTask(fn func() error) (*BackgroundTask, error) {
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
