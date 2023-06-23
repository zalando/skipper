package openpolicyagent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/config"
	"github.com/open-policy-agent/opa/logging"
	"github.com/open-policy-agent/opa/plugins"
	"github.com/open-policy-agent/opa/plugins/discovery"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/runtime"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/storage/inmem"
	iCache "github.com/open-policy-agent/opa/topdown/cache"
	opautil "github.com/open-policy-agent/opa/util"
	"github.com/opentracing/opentracing-go"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/envoy"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/util"
)

const (
	OpenPolicyAgentDecisionKey        = "open-policy-agent:decision"
	OpenPolicyAgentDecisionBodyKey    = "open-policy-agent:decision-body"
	OpenPolicyAgentDecisionHeadersKey = "open-policy-agent:decision-headers"
)

type OpenPolicyAgentFactory struct {
	// Ideally share one Bundle storage across many OPA "instances" using this factory.
	// This allows to save memory on bundles that are shared
	// between different policies (i.e. global team memberships)
	// This not possible due to some limitations in OPA
	// See https://github.com/open-policy-agent/opa/issues/5707
}

func NewOpenPolicyAgentFactory() OpenPolicyAgentFactory {
	return OpenPolicyAgentFactory{}
}

type OpenPolicyAgentInstanceConfig struct {
	envoyMetadata          *ext_authz_v3_core.Metadata
	configTemplate         []byte
	envoyContextExtensions map[string]string
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

func WithEnvoyContextExtensions(envoyContextExtensions map[string]string) func(*OpenPolicyAgentInstanceConfig) error {
	return func(cfg *OpenPolicyAgentInstanceConfig) error {
		cfg.envoyContextExtensions = envoyContextExtensions
		return nil
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
			return fmt.Errorf("cannot parse '%v': %w", file, err)
		}

		return nil
	}
}

func (cfg *OpenPolicyAgentInstanceConfig) GetEnvoyMetadata() *ext_authz_v3_core.Metadata {
	return cfg.envoyMetadata
}

func (cfg *OpenPolicyAgentInstanceConfig) GetEnvoyContextExtensions() map[string]string {
	return cfg.envoyContextExtensions
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
		cfg.configTemplate, err = os.ReadFile(configTemplate())
		if err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

func configTemplate() string {
	if s, ok := os.LookupEnv("OPA_CONFIG_TEMPLATE_FILE"); ok {
		return s
	}
	return "opaconfig.yaml"
}

func (factory *OpenPolicyAgentFactory) NewOpenPolicyAgentInstance(bundleName string, config OpenPolicyAgentInstanceConfig, filterName string) (*OpenPolicyAgentInstance, error) {
	runtime.RegisterPlugin(envoy.PluginName, envoy.Factory{})

	configBytes, err := interpolateConfigTemplate(config.configTemplate, bundleName)
	if err != nil {
		return nil, err
	}

	engine, err := New(inmem.New(), configBytes, config, filterName, bundleName)

	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err = engine.Start(ctx); err != nil {
		return nil, err
	}

	err = engine.waitPluginsReady(100*time.Millisecond, 30*time.Second)
	if err != nil {
		engine.Logger().WithFields(map[string]interface{}{"err": err}).Error("Failed to wait for plugins activation.")
		return nil, err
	}

	return engine, nil
}

type OpenPolicyAgentInstance struct {
	manager        *plugins.Manager
	instanceConfig OpenPolicyAgentInstanceConfig
	opaConfig      *config.Config
	bundleName     string

	preparedQuery          *rego.PreparedEvalQuery
	preparedQueryDoOnce    *sync.Once
	interQueryBuiltinCache iCache.InterQueryCache
}

func envVariablesMap() map[string]string {
	var rawEnvVariables = os.Environ()

	var envVariables = make(map[string]string)

	for _, item := range rawEnvVariables {
		tokens := strings.SplitN(item, "=", 2)

		envVariables[tokens[0]] = tokens[1]
	}

	return envVariables
}

// Config sets the configuration file to use on the OPA instance.
func interpolateConfigTemplate(configTemplate []byte, bundleName string) ([]byte, error) {
	var buf bytes.Buffer

	tpl := template.Must(template.New("opa-config").Parse(string(configTemplate)))

	binding := make(map[string]interface{})
	binding["bundlename"] = bundleName
	binding["Env"] = envVariablesMap()

	err := tpl.ExecuteTemplate(&buf, "opa-config", binding)

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// New returns a new OPA object.
func New(store storage.Store, configBytes []byte, instanceConfig OpenPolicyAgentInstanceConfig, filterName string, bundleName string) (*OpenPolicyAgentInstance, error) {
	id, err := util.Uuid4()
	if err != nil {
		return nil, err
	}

	opaConfig, err := config.ParseConfig(configBytes, id)
	if err != nil {
		return nil, err
	}

	runtime.RegisterPlugin(envoy.PluginName, envoy.Factory{})

	var logger logging.Logger = &QuietLogger{target: logging.Get()}
	logger = logger.WithFields(map[string]interface{}{"skipper-filter": filterName})
	manager, err := plugins.New(configBytes, id, store, plugins.Logger(logger))
	if err != nil {
		return nil, err
	}

	discovery, err := discovery.New(manager, discovery.Factories(map[string]plugins.Factory{envoy.PluginName: envoy.Factory{}}))
	if err != nil {
		return nil, err
	}

	manager.Register("discovery", discovery)

	opa := &OpenPolicyAgentInstance{
		instanceConfig: instanceConfig,
		manager:        manager,
		opaConfig:      opaConfig,
		bundleName:     bundleName,

		preparedQueryDoOnce:    new(sync.Once),
		interQueryBuiltinCache: iCache.NewInterQueryCache(manager.InterQueryBuiltinCacheConfig()),
	}

	manager.RegisterCompilerTrigger(opa.compilerUpdated)

	return opa, nil
}

// Start asynchronously starts the policy engine's plugins that download
// policies, report status, etc.
func (opa *OpenPolicyAgentInstance) Start(ctx context.Context) error {
	return opa.manager.Start(ctx)
}

func (opa *OpenPolicyAgentInstance) waitPluginsReady(checkInterval, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
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

	opa.Logger().Debug("Waiting for plugins activation (%v).", timeout)

	return opautil.WaitFunc(pluginsReady, checkInterval, timeout)
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

func (opa *OpenPolicyAgentInstance) startSpanFromContextWithTracer(tr opentracing.Tracer, parent opentracing.Span, ctx context.Context) (opentracing.Span, context.Context) {
	var span opentracing.Span
	if parent != nil {
		span = tr.StartSpan("openpolicyagent", opentracing.ChildOf(parent.Context()))
	} else {
		span = tr.StartSpan("openpolicyagent")
	}

	span.SetTag("opa.bundle_name", opa.bundleName)

	for label, value := range opa.manager.Labels() {
		span.SetTag("opa.label."+label, value)
	}

	return span, opentracing.ContextWithSpan(ctx, span)
}

func (opa *OpenPolicyAgentInstance) StartSpanFromFilterContext(fc filters.FilterContext) (opentracing.Span, context.Context) {
	return opa.startSpanFromContextWithTracer(fc.Tracer(), fc.ParentSpan(), fc.Request().Context())
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

// Implementation of the envoyauth.EvalContext interface
func (opa *OpenPolicyAgentInstance) ParsedQuery() ast.Body {
	return opa.EnvoyPluginConfig().ParsedQuery
}

func (opa *OpenPolicyAgentInstance) Store() storage.Store            { return opa.manager.Store }
func (opa *OpenPolicyAgentInstance) Compiler() *ast.Compiler         { return opa.manager.GetCompiler() }
func (opa *OpenPolicyAgentInstance) Runtime() *ast.Term              { return opa.manager.Info }
func (opa *OpenPolicyAgentInstance) Logger() logging.Logger          { return opa.manager.Logger() }
func (opa *OpenPolicyAgentInstance) PreparedQueryDoOnce() *sync.Once { return opa.preparedQueryDoOnce }
func (opa *OpenPolicyAgentInstance) InterQueryBuiltinCache() iCache.InterQueryCache {
	return opa.interQueryBuiltinCache
}
func (opa *OpenPolicyAgentInstance) PreparedQuery() *rego.PreparedEvalQuery { return opa.preparedQuery }
func (opa *OpenPolicyAgentInstance) SetPreparedQuery(q *rego.PreparedEvalQuery) {
	opa.preparedQuery = q
}
func (opa *OpenPolicyAgentInstance) Config() *config.Config {
	return opa.opaConfig
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
