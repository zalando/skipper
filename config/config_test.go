package config

import (
	"crypto/tls"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy"
	"gopkg.in/yaml.v2"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestEnvOverrides_SwarmRedisPassword(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		env  string
		want string
	}{
		{
			name: "don't set redis password either from file nor environment",
			args: []string{"skipper"},
			env:  "",
			want: "",
		},
		{
			name: "set redis password from environment",
			args: []string{"skipper"},
			env:  "set_from_env",
			want: "set_from_env",
		},
		{
			name: "set redis password from config file and ignore environment",
			args: []string{"skipper", "-config-file=testdata/test.yaml"},
			env:  "set_from_env",
			want: "set_from_file",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(redisPasswordEnv, tt.env)
			}
			cfg := NewConfig()
			err := cfg.ParseArgs(tt.args[0], tt.args[1:])
			if err != nil {
				t.Errorf("config.NewConfig() error = %v", err)
			}

			if cfg.SwarmRedisPassword != tt.want {
				t.Errorf("cfg.SwarmRedisPassword didn't set correctly: Want '%s', got '%s'", tt.want, cfg.SwarmRedisPassword)
			}
		})
	}
}

func defaultConfig(with func(*Config)) *Config {
	cfg := &Config{
		Flags:                                   nil,
		Address:                                 ":9090",
		StatusChecks:                            commaListFlag(),
		ExpectedBytesPerRequest:                 50 * 1024,
		SupportListener:                         ":9911",
		MaxLoopbacks:                            proxy.DefaultMaxLoopbacks,
		DefaultHTTPStatus:                       404,
		MaxAuditBody:                            1024,
		MaxMatcherBufferSize:                    2097152,
		MetricsFlavour:                          commaListFlag("codahale", "prometheus"),
		FilterPlugins:                           newPluginFlag(),
		PredicatePlugins:                        newPluginFlag(),
		DataclientPlugins:                       newPluginFlag(),
		MultiPlugins:                            newPluginFlag(),
		CompressEncodings:                       commaListFlag("gzip", "deflate", "br"),
		OpenTracing:                             "noop",
		OpenTracingInitialSpan:                  "ingress",
		OpentracingLogFilterLifecycleEvents:     true,
		OpentracingLogStreamEvents:              true,
		MetricsListener:                         ":9911",
		MetricsPrefix:                           "skipper.",
		RuntimeMetrics:                          true,
		HistogramMetricBuckets:                  []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		ApplicationLogLevel:                     log.InfoLevel,
		ApplicationLogLevelString:               "INFO",
		ApplicationLogPrefix:                    "[APP]",
		EtcdPrefix:                              "/skipper",
		EtcdTimeout:                             time.Second,
		AppendFilters:                           &defaultFiltersFlags{},
		PrependFilters:                          &defaultFiltersFlags{},
		DisabledFilters:                         commaListFlag(),
		CloneRoute:                              routeChangerConfig{},
		EditRoute:                               routeChangerConfig{},
		SourcePollTimeout:                       3000,
		KubernetesEastWestRangeDomains:          commaListFlag(),
		KubernetesHealthcheck:                   true,
		KubernetesHTTPSRedirect:                 true,
		KubernetesHTTPSRedirectCode:             308,
		KubernetesPathModeString:                "kubernetes-ingress",
		KubernetesRedisServicePort:              6379,
		KubernetesBackendTrafficAlgorithmString: "traffic-predicate",
		KubernetesDefaultLoadBalancerAlgorithm:  "roundRobin",
		Oauth2TokeninfoTimeout:                  2 * time.Second,
		Oauth2TokenintrospectionTimeout:         2 * time.Second,
		Oauth2TokeninfoSubjectKey:               "uid",
		Oauth2GrantTokeninfoKeys:                commaListFlag(),
		Oauth2TokenCookieName:                   "oauth2-grant",
		Oauth2TokenCookieRemoveSubdomains:       1,
		WebhookTimeout:                          2 * time.Second,
		OidcDistributedClaimsTimeout:            2 * time.Second,
		OIDCCookieValidity:                      time.Hour,
		OIDCCookieRemoveSubdomains:              1,
		CredentialPaths:                         commaListFlag(),
		CredentialsUpdateInterval:               10 * time.Minute,
		ApiUsageMonitoringClientKeys:            "sub",
		ApiUsageMonitoringRealmsTrackingPattern: "services",
		WaitForHealthcheckInterval:              45 * time.Second,
		IdleConnsPerHost:                        64,
		CloseIdleConnsPeriod:                    20 * time.Second,
		BackendFlushInterval:                    20 * time.Millisecond,
		ReadTimeoutServer:                       5 * time.Minute,
		ReadHeaderTimeoutServer:                 1 * time.Minute,
		WriteTimeoutServer:                      1 * time.Minute,
		IdleTimeoutServer:                       1 * time.Minute,
		MaxHeaderBytes:                          1048576,
		TimeoutBackend:                          1 * time.Minute,
		KeepaliveBackend:                        30 * time.Second,
		EnableDualstackBackend:                  true,
		TlsHandshakeTimeoutBackend:              1 * time.Minute,
		ResponseHeaderTimeoutBackend:            1 * time.Minute,
		ExpectContinueTimeoutBackend:            30 * time.Second,
		ServeMethodMetric:                       true,
		ServeStatusCodeMetric:                   true,
		SwarmRedisURLs:                          commaListFlag(),
		SwarmRedisDialTimeout:                   25 * time.Millisecond,
		SwarmRedisReadTimeout:                   25 * time.Millisecond,
		SwarmRedisWriteTimeout:                  25 * time.Millisecond,
		SwarmRedisPoolTimeout:                   25 * time.Millisecond,
		SwarmRedisMinIdleConns:                  100,
		SwarmRedisMaxIdleConns:                  100,
		SwarmKubernetesNamespace:                "kube-system",
		SwarmKubernetesLabelSelectorKey:         "application",
		SwarmKubernetesLabelSelectorValue:       "skipper-ingress",
		SwarmPort:                               9990,
		SwarmMaxMessageBuffer:                   4194304,
		SwarmLeaveTimeout:                       5 * time.Second,
		TLSMinVersion:                           defaultMinTLSVersion,
		RoutesURLs:                              commaListFlag(),
		ForwardedHeadersList:                    commaListFlag(),
		ForwardedHeadersExcludeCIDRList:         commaListFlag(),
		ClusterRatelimitMaxGroupShards:          1,
		ValidateQuery:                           true,
		ValidateQueryLog:                        true,
		LuaModules:                              commaListFlag(),
		LuaSources:                              commaListFlag(),
		OpenPolicyAgentCleanerInterval:          openpolicyagent.DefaultCleanIdlePeriod,
		OpenPolicyAgentStartupTimeout:           openpolicyagent.DefaultOpaStartupTimeout,
		OpenPolicyAgentControlLoopInterval:      openpolicyagent.DefaultControlLoopInterval,
		OpenPolicyAgentControlLoopMaxJitter:     openpolicyagent.DefaultControlLoopMaxJitter,
		OpenPolicyAgentMaxRequestBodySize:       openpolicyagent.DefaultMaxRequestBodySize,
		OpenPolicyAgentMaxMemoryBodyParsing:     openpolicyagent.DefaultMaxMemoryBodyParsing,
		OpenPolicyAgentRequestBodyBufferSize:    openpolicyagent.DefaultRequestBodyBufferSize,
	}
	with(cfg)
	return cfg
}

func TestToOptions(t *testing.T) {
	c := defaultConfig(func(c *Config) {
		// ProxyFlags
		c.Insecure = true          // 1
		c.ProxyPreserveHost = true // 4
		c.RemoveHopHeaders = true  // 16
		c.RfcPatchPath = true      // 32
		c.ExcludeInsecureCipherSuites = true

		// config
		c.EtcdUrls = "127.0.0.1:5555"
		c.WhitelistedHealthCheckCIDR = "127.0.0.0/8,10.0.0.0/8"
		c.ForwardedHeadersList = commaListFlag("X-Forwarded-For,X-Forwarded-Host,X-Forwarded-Method,X-Forwarded-Uri,X-Forwarded-Port=,X-Forwarded-Proto=http")
		c.ForwardedHeadersList.Set("X-Forwarded-For,X-Forwarded-Host,X-Forwarded-Method,X-Forwarded-Uri,X-Forwarded-Port=,X-Forwarded-Proto=http")
		c.HostPatch = net.HostPatch{
			ToLower:           true,
			RemoteTrailingDot: true,
		}
		c.RefusePayload = append(c.RefusePayload, "refuse")
		c.ValidateQuery = true
		c.ValidateQueryLog = true

		c.CloneRoute = routeChangerConfig{}
		if err := c.CloneRoute.Set("/foo/bar/"); err != nil {
			t.Fatalf("Failed to set: %v", err)
		}
		c.EditRoute = routeChangerConfig{}
		if err := c.EditRoute.Set("/foo/bar/"); err != nil {
			t.Fatalf("Failed to set: %v", err)
		}
	})

	if err := validate(c); err != nil {
		t.Fatalf("Failed to validate config: %v", err)
	}
	opt := c.ToOptions()

	// validate
	if !c.HostPatch.ToLower {
		t.Error("Failed to set HostPatch ToLower")
	}
	if !c.HostPatch.RemoteTrailingDot {
		t.Error("Failed to set HostPatch RemoteTrailingDot")
	}
	if opt.ProxyFlags != proxy.Flags(2+8+32+64) {
		t.Errorf("Failed to get ProxyFlags: %v", opt.ProxyFlags)
	}
	if opt.CustomHttpHandlerWrap == nil {
		t.Errorf("Failed to get Forwarded Wrappers: %p", opt.CustomHttpHandlerWrap)
	}
	if opt.AccessLogDisabled {
		t.Error("Failed to get options AccessLogDisabled")
	}
	if len(opt.EtcdUrls) != 1 {
		t.Errorf("Failed to get EtcdUrls: %v", opt.EtcdUrls)
	}
	if len(opt.WhitelistedHealthCheckCIDR) != 2 {
		t.Errorf("Failed to get WhitelistedHealthCheckCIDR: %v", opt.WhitelistedHealthCheckCIDR)
	}
	if len(opt.CloneRoute) != 1 {
		t.Errorf("Failed to get expected clone route: %s", c.CloneRoute)
	}
	if len(opt.EditRoute) != 1 {
		t.Errorf("Failed to get expected edit route: %s", c.EditRoute)
	}
	if opt.CipherSuites == nil {
		t.Errorf("Failed to get the filtered cipher suites")
	} else {
		for _, i := range tls.InsecureCipherSuites() {
			if slices.Contains(opt.CipherSuites, i.ID) {
				t.Errorf("Insecure cipher found in list: %s", i.Name)
			}
		}
	}
}

func Test_Validate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		change  func(c *Config)
		want    error
		wantErr bool
	}{
		{
			name: "test wrong loglevel",
			change: func(c *Config) {
				c.ApplicationLogLevelString = "wrongLevel"
			},
			want:    errors.New(`not a valid logrus Level: "wrongLevel"`),
			wantErr: true,
		},
		{
			name: "test valid config",
			change: func(c *Config) {
				c.HistogramMetricBucketsString = ""
				c.ApplicationLogLevel = log.InfoLevel
				c.ApplicationLogLevelString = "INFO"
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test wrong KubernetesPathModeString",
			change: func(c *Config) {
				c.KubernetesPathModeString = "wrongPathMode"
			},
			wantErr: true,
			want:    errors.New(`invalid path mode string: wrongPathMode`),
		},
		{
			name: "test wrong KubernetesEastWestRangePredicatesString",
			change: func(c *Config) {
				c.KubernetesEastWestRangePredicatesString = "WrongEastWestMode"
			},
			wantErr: true,
			want:    errors.New("invalid east-west-range-predicates: parse failed after token WrongEastWestMode, position 17: syntax error"),
		},
		{
			name: "test wrong HistoGramBuckets",
			change: func(c *Config) {
				c.HistogramMetricBucketsString = "5,10,abc"
			},
			wantErr: true,
			want:    errors.New(`unable to parse histogram-metric-buckets: strconv.ParseFloat: parsing "abc": invalid syntax`),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig()
			tt.change(cfg)
			err := validate(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("config.NewConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && err.Error() != tt.want.Error() {
				t.Errorf("Failed to get wanted error, got: %v, want: %v", err, tt.want)
			}
		})
	}
}

func Test_NewConfigWithArgs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		want    *Config
		wantErr bool
	}{
		{
			name:    "test args len bigger than 0 throws an error",
			args:    []string{"skipper", "arg1"},
			wantErr: true,
		},
		{
			name:    "test non-existing config file throw an error",
			args:    []string{"skipper", "-config-file=non-existent.yaml"},
			wantErr: true,
		},
		{
			name: "test only valid flag overwrite yaml file",
			args: []string{"skipper", "-config-file=testdata/test.yaml", "-address=localhost:8080", "-refuse-payload=baz"},
			want: defaultConfig(func(c *Config) {
				c.ConfigFile = "testdata/test.yaml"
				c.Address = "localhost:8080"
				c.MaxLoopbacks = 12
				c.EtcdTimeout = 2 * time.Second
				c.StatusChecks = &listFlag{
					sep:     ",",
					allowed: map[string]bool{},
					value:   "http://foo.test/bar,http://baz.test/qux",
					values:  []string{"http://foo.test/bar", "http://baz.test/qux"},
				}
				c.SwarmRedisPassword = "set_from_file"
				// Flags specified in test.yaml are zeroed out if not provided in args again
				c.SwarmRedisEndpointsUpdateInterval = 0
				c.SwarmRedisConnMetricsInterval = 0
				c.SwarmRedisMetricsPrefix = ""
				c.RefusePayload = multiFlag{"foo", "bar", "baz"}
			}),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig()
			err := cfg.ParseArgs(tt.args[0], tt.args[1:])

			if (err != nil) != tt.wantErr {
				t.Fatalf("config.NewConfig() error: %v, wantErr: %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				d := cmp.Diff(tt.want, cfg,
					cmp.AllowUnexported(listFlag{}, pluginFlag{}, defaultFiltersFlags{}, mapFlags{}),
					cmpopts.IgnoreUnexported(Config{}), cmpopts.IgnoreFields(Config{}, "Flags"),
				)
				if d != "" {
					t.Errorf("config.NewConfig() want vs got:\n%s", d)
				}
			}
		})
	}
}

func Test_parseHistogramBuckets(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    string
		want    []float64
		wantErr bool
	}{
		{
			name:    "test parse 1",
			args:    "1",
			want:    []float64{1},
			wantErr: false,
		},
		{
			name:    "test parse 1,1.33,1.5,1.66,2",
			args:    "1,1.33,1.5,1.66,2",
			want:    []float64{1, 1.33, 1.5, 1.66, 2},
			wantErr: false,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := new(Config)
			cfg.HistogramMetricBucketsString = tt.args

			got, err := cfg.parseHistogramBuckets()
			if !reflect.DeepEqual(got, tt.want) || (tt.wantErr && err == nil) || (!tt.wantErr && err != nil) {
				t.Errorf("Failed to parse histogram buckets: Want %v, got %v, err %v", tt.want, got, err)
			}
		})
	}
}

func TestMinTLSVersion(t *testing.T) {
	t.Run("test default", func(t *testing.T) {
		cfg := new(Config)
		if cfg.getMinTLSVersion() != tls.VersionTLS12 {
			t.Error("Failed to get default min TLS version")
		}
	})
	t.Run("test configured TLS version", func(t *testing.T) {
		cfg := new(Config)
		cfg.TLSMinVersion = "1.3"
		if cfg.getMinTLSVersion() != tls.VersionTLS13 {
			t.Error(`Failed to get correct TLS version for "1.3"`)
		}
		cfg.TLSMinVersion = "11"
		if cfg.getMinTLSVersion() != tls.VersionTLS11 {
			t.Error(`Failed to get correct TLS version for "11"`)
		}
	})
}

func TestExcludeInsecureCipherSuites(t *testing.T) {

	t.Run("test default", func(t *testing.T) {
		cfg := new(Config)
		if cfg.filterCipherSuites() != nil {
			t.Error("No cipher suites should be filtered by default")
		}
	})
	t.Run("test excluded insecure cipher suites", func(t *testing.T) {
		cfg := new(Config)
		cfg.ExcludeInsecureCipherSuites = true
		if cfg.filterCipherSuites() == nil {
			t.Error(`Failed to get list of filtered cipher suites`)
		}
	})
}

type testFormatter struct {
	messages map[string]log.Level
}

func (tf *testFormatter) Format(entry *log.Entry) ([]byte, error) {
	tf.messages[entry.Message] = entry.Level
	return nil, nil
}

func TestDeprecatedFlags(t *testing.T) {
	o := log.StandardLogger().Out
	f := log.StandardLogger().Formatter
	defer func() {
		log.SetOutput(o)
		log.SetFormatter(f)
	}()

	formatter := &testFormatter{messages: make(map[string]log.Level)}

	args := []string{
		"skipper",
		"-config-file=testdata/deprecated.yaml",
		"-enable-prometheus-metrics=false", // default value should produce deprecation warning as well
		"-api-usage-monitoring-default-client-tracking-pattern=whatever",
	}
	log.SetOutput(os.Stdout)
	log.SetFormatter(formatter)

	cfg := NewConfig()
	err := cfg.ParseArgs(args[0], args[1:])
	if err != nil {
		t.Fatal(err)
	}

	t.Log(formatter.messages)

	if len(formatter.messages) != 4 {
		t.Errorf("expected 4 deprecation warnings, got %d", len(formatter.messages))
	}
	for message, level := range formatter.messages {
		if level != log.WarnLevel {
			t.Errorf("warn level expected, got %v for %q", level, message)
		}
		if !strings.Contains(message, "*Deprecated*") {
			t.Errorf("Deprecated marker expected for %q", message)
		}
	}
}

func TestBuildRedisTLSConfig(t *testing.T) {
	// Create temporary test certificate files
	tempDir := t.TempDir()

	// Valid CA cert content (minimal valid cert for testing)
	caCertPEM := `-----BEGIN CERTIFICATE-----
MIICBjCCAW+gAwIBAgIJAMlyFqk69v+9MA0GCSqGSIb3DQEBCwUAMBIxEDAOBgNV
BAMMB1Rlc3QtQ0EwHhcNMjMwMTAxMDAwMDAwWhcNMzMwMTAxMDAwMDAwWjASMRAw
DgYDVQQDDAdUZXN0LUNBMIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDTGX9l
GXh8J9T4E3i5IZ6lfF/NUlhYzxBFJzR6h+6k9QXV5oK5HqJXg9M5bE6iT1d0eD8Q
9bB4k5j6L8I9bB4k5j6L8I9bB4k5j6L8I9bB4k5j6L8I9bB4k5j6L8I9bB4k5j6L
8I9bB4k5j6L8I9bB4k5j6L8I9bB4k5j6L8I9bB4k5j6L8I9bB4k5wIDAQABMA0G
CSqGSIb3DQEBCwUAA4GBAJKvSzpUGWVQG7n0u6g2dH1+lQ3W8W7i8Y4E7n0u6g2d
H1+lQ3W8W7i8Y4E7n0u6g2dH1+lQ3W8W7i8Y4E7n0u6g2dH1+lQ3W8W7i8Y4E7
-----END CERTIFICATE-----`

	// Valid client cert content (minimal valid cert for testing)
	clientCertPEM := `-----BEGIN CERTIFICATE-----
MIICBjCCAW+gAwIBAgIJAMlyFqk69v+9MA0GCSqGSIb3DQEBCwUAMBIxEDAOBgNV
BAMMB0NsaWVudDAeFw0yMzAxMDEwMDAwMDBaFw0zMzAxMDEwMDAwMDBaMBIxEDAw
DgYDVQQDDAdDbGllbnQwgZ8wDQYJKoZIhvcNAQEBBQADgY0AMIGJAoGBANMZf2UZ
eHwn1PgTeLkhnqV8X81SWFjPEEUnNHqH7qT1BdXmgrkeolePzTlsTqJPV3R4PxD1
sHiTmPovwj1sHiTmPovwj1sHiTmPovwj1sHiTmPovwj1sHiTmPovwj1sHiTmPovw
j1sHiTmPovwj1sHiTmPovwj1sHiTmPovwj1sHiTmPovwj1sHiTnAgMBAAEwDQYJ
KoZIhvcNAQELBQADgYEAkq9LOlQZZVAbufS7qDZ0fX6VDdbxbuLxjgTufS7qDZ0f
X6VDdbxbuLxjgTufS7qDZ0fX6VDdbxbuLxjgTufS7qDZ0fX6VDdbxbuLxjgTufS7
qDZ0fX6VDdbxbuLxjgTufS7qDZ0fX6VDdbxbuLxjgTufS7qDZ0fX6VDdbxbuLxjg
-----END CERTIFICATE-----`

	// Valid client key content (minimal valid key for testing)
	clientKeyPEM := `-----BEGIN PRIVATE KEY-----
MIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBANMZf2UZeHwn1PgT
eLkhnqV8X81SWFjPEEUnNHqH7qT1BdXmgrkeole/zTlsTqJPV3R4PxD1sHiTmPov
wj1sHiTmPovwj1sHiTmPovwj1sHiTmPovwj1sHiTmPovwj1sHiTmPovwj1sHiTmP
ovwj1sHiTmPovwj1sHiTmPovwj1sHiTmPovwj1sHiTnAgMBAAECgYEAuR1VQMGW
Dqk17v2TLnOHvn6ZrFe2+XjLZWlO7v2TLnOHvn6ZrFe2+XjLZWlO7v2TLnOHvn6Z
rFe2+XjLZWlO7v2TLnOHvn6ZrFe2+XjLZWlO7v2TLnOHvn6ZrFe2+XjLZWlO7v2T
LnOHvn6ZrFe2+XjLZWlO7v2TLnOHvn6ZrFeCggEBAPuWYE9GZvWrF7t+B4qA3+8v
YE9GZvWrF7t+B4qA3+8vYE9GZvWrF7t+B4qA3+8vYE9GZvWrF7t+B4qA3+8vwIBA
+8vYE9GZvWrF7t+B4qA3+8vYE9GZvWrF7t+B4qA3+8vwIBAgIVAJQ4k6k3Lv2T8
I+8vYE9GZvWrF7t+B4qA3+8vYE9GZvWrF7t+B4qA3+8vwwKBgGWlO7v2TLnOHvn6
ZrFe2+XjLZWlO7v2TLnOHvn6ZrFe2+XjLZWlO7v2TLnOHvn6ZrFe2+XjLZWlO7v2
TLnOHvn6ZrFe2+XjLZWlO7v2TLnOHvn6ZrFe2+XjLZWlO7v2TLnOHvn6ZrFe2+Xj
-----END PRIVATE KEY-----`

	// Create test files
	caCertPath := tempDir + "/ca.crt"
	clientCertPath := tempDir + "/client.crt"
	clientKeyPath := tempDir + "/client.key"
	invalidCertPath := tempDir + "/invalid.crt"

	require.NoError(t, os.WriteFile(caCertPath, []byte(caCertPEM), 0644))
	require.NoError(t, os.WriteFile(clientCertPath, []byte(clientCertPEM), 0644))
	require.NoError(t, os.WriteFile(clientKeyPath, []byte(clientKeyPEM), 0644))
	require.NoError(t, os.WriteFile(invalidCertPath, []byte("invalid cert data"), 0644))

	tests := []struct {
		name   string
		config Config
		want   bool // true if TLS config should be returned, false for nil
	}{
		{
			name: "TLS disabled",
			config: Config{
				SwarmRedisTLSEnabled: false,
			},
			want: false,
		},
		{
			name: "TLS enabled with server name only",
			config: Config{
				SwarmRedisTLSEnabled:    true,
				SwarmRedisTLSServerName: "redis.example.com",
				TLSMinVersion:           "1.2",
			},
			want: true,
		},
		{
			name: "TLS enabled with insecure skip verify",
			config: Config{
				SwarmRedisTLSEnabled:            true,
				SwarmRedisTLSInsecureSkipVerify: true,
				TLSMinVersion:                   "1.2",
			},
			want: true,
		},
		{
			name: "TLS enabled with invalid CA cert path",
			config: Config{
				SwarmRedisTLSEnabled:    true,
				SwarmRedisTLSCACertPath: "/nonexistent/ca.crt",
				TLSMinVersion:           "1.2",
			},
			want: false,
		},
		{
			name: "TLS enabled with invalid CA cert content",
			config: Config{
				SwarmRedisTLSEnabled:    true,
				SwarmRedisTLSCACertPath: invalidCertPath,
				TLSMinVersion:           "1.2",
			},
			want: false,
		},
		{
			name: "TLS enabled with invalid client cert path",
			config: Config{
				SwarmRedisTLSEnabled:  true,
				SwarmRedisTLSCertPath: "/nonexistent/client.crt",
				SwarmRedisTLSKeyPath:  clientKeyPath,
				TLSMinVersion:         "1.2",
			},
			want: false,
		},
		{
			name: "TLS enabled with only cert path (missing key)",
			config: Config{
				SwarmRedisTLSEnabled:  true,
				SwarmRedisTLSCertPath: clientCertPath,
				TLSMinVersion:         "1.2",
			},
			want: true, // Should return config but warn about missing key
		},
		{
			name: "TLS enabled with only key path (missing cert)",
			config: Config{
				SwarmRedisTLSEnabled: true,
				SwarmRedisTLSKeyPath: clientKeyPath,
				TLSMinVersion:        "1.2",
			},
			want: true, // Should return config but warn about missing cert
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsConfig := tt.config.buildRedisTLSConfig()

			if tt.want {
				require.NotNil(t, tlsConfig, "Expected TLS config to be returned")
				assert.Equal(t, tt.config.SwarmRedisTLSServerName, tlsConfig.ServerName)
				assert.Equal(t, tt.config.SwarmRedisTLSInsecureSkipVerify, tlsConfig.InsecureSkipVerify)
				assert.Equal(t, tt.config.getMinTLSVersion(), tlsConfig.MinVersion)
			} else {
				assert.Nil(t, tlsConfig, "Expected TLS config to be nil")
			}
		})
	}
}

func TestBuildRedisOptions(t *testing.T) {
	tests := []struct {
		name           string
		config         Config
		expectOptions  bool
		expectedFields map[string]interface{}
	}{
		{
			name: "no Redis URLs or remote URL",
			config: Config{
				SwarmRedisURLs: &listFlag{},
			},
			expectOptions: false,
		},
		{
			name: "with Redis URLs only",
			config: Config{
				SwarmRedisURLs:        &listFlag{values: []string{"redis://localhost:6379"}},
				SwarmRedisPassword:    "test-password",
				SwarmRedisClusterMode: true,
				SwarmRedisDialTimeout: 30 * time.Second,
			},
			expectOptions: true,
			expectedFields: map[string]interface{}{
				"Password":    "test-password",
				"ClusterMode": true,
				"DialTimeout": 30 * time.Second,
			},
		},
		{
			name: "with remote URL only",
			config: Config{
				SwarmRedisURLs:                    &listFlag{},
				SwarmRedisEndpointsRemoteURL:      "http://example.com/redis-endpoints",
				SwarmRedisEndpointsUpdateInterval: 5 * time.Minute,
			},
			expectOptions: true,
			expectedFields: map[string]interface{}{
				"RemoteURL":      "http://example.com/redis-endpoints",
				"UpdateInterval": 5 * time.Minute,
			},
		},
		{
			name: "cluster mode with remote URL (should warn)",
			config: Config{
				SwarmRedisURLs:               &listFlag{values: []string{"redis://localhost:6379"}},
				SwarmRedisEndpointsRemoteURL: "http://example.com/redis-endpoints",
				SwarmRedisClusterMode:        true,
			},
			expectOptions: true,
			expectedFields: map[string]interface{}{
				"ClusterMode": true,
				"RemoteURL":   "http://example.com/redis-endpoints", // Should be set but ignored
			},
		},
		{
			name: "all options configured",
			config: Config{
				SwarmRedisURLs:                &listFlag{values: []string{"redis://localhost:6379", "redis://localhost:6380"}},
				SwarmRedisPassword:            "password123",
				SwarmRedisClusterMode:         false,
				SwarmRedisHashAlgorithm:       "jump",
				SwarmRedisDialTimeout:         25 * time.Millisecond,
				SwarmRedisReadTimeout:         30 * time.Millisecond,
				SwarmRedisWriteTimeout:        35 * time.Millisecond,
				SwarmRedisPoolTimeout:         40 * time.Millisecond,
				SwarmRedisIdleTimeout:         5 * time.Minute,
				SwarmRedisMaxConnAge:          10 * time.Minute,
				SwarmRedisMinIdleConns:        50,
				SwarmRedisMaxIdleConns:        200,
				SwarmRedisHeartbeatFrequency:  1 * time.Second,
				SwarmRedisConnMetricsInterval: 30 * time.Second,
				SwarmRedisMetricsPrefix:       "test_prefix",
				SwarmRedisIdleCheckFrequency:  1 * time.Minute,
				SwarmRedisTLSEnabled:          true,
				SwarmRedisTLSServerName:       "redis.example.com",
				TLSMinVersion:                 "1.2",
			},
			expectOptions: true,
			expectedFields: map[string]interface{}{
				"Password":            "password123",
				"ClusterMode":         false,
				"HashAlgorithm":       "jump",
				"DialTimeout":         25 * time.Millisecond,
				"ReadTimeout":         30 * time.Millisecond,
				"WriteTimeout":        35 * time.Millisecond,
				"PoolTimeout":         40 * time.Millisecond,
				"IdleTimeout":         5 * time.Minute,
				"MaxConnAge":          10 * time.Minute,
				"MinIdleConns":        50,
				"MaxIdleConns":        200,
				"HeartbeatFrequency":  1 * time.Second,
				"ConnMetricsInterval": 30 * time.Second,
				"MetricsPrefix":       "test_prefix",
				"IdleCheckFrequency":  1 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.buildRedisOptions()

			if tt.expectOptions {
				require.NotNil(t, tt.config.RedisOptionsForRatelimit, "Expected Redis options to be built")

				// Check specific fields if provided
				if tt.expectedFields != nil {
					ro := tt.config.RedisOptionsForRatelimit
					roValue := reflect.ValueOf(ro).Elem()

					for fieldName, expectedValue := range tt.expectedFields {
						field := roValue.FieldByName(fieldName)
						require.True(t, field.IsValid(), "Field %s should exist", fieldName)
						assert.Equal(t, expectedValue, field.Interface(), "Field %s should match expected value", fieldName)
					}
				}

				// Check that Addrs are properly copied
				if len(tt.config.SwarmRedisURLs.values) > 0 {
					assert.Equal(t, tt.config.SwarmRedisURLs.values, tt.config.RedisOptionsForRatelimit.Addrs)
				}

				// Check that TLS config is included if enabled
				if tt.config.SwarmRedisTLSEnabled {
					assert.NotNil(t, tt.config.RedisOptionsForRatelimit.TLSConfig)
				}
			} else {
				assert.Nil(t, tt.config.RedisOptionsForRatelimit, "Expected Redis options to be nil")
			}
		})
	}
}

func TestMultiFlagYamlErr(t *testing.T) {
	m := &multiFlag{}
	err := yaml.Unmarshal([]byte(`foo=bar`), m)
	if err == nil {
		t.Error("Failed to get error on wrong yaml input")
	}
}

func TestParseAnnotationConfigInvalid(t *testing.T) {
	t.Run("parseAnnotationPredicates", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			input []string
		}{
			{
				name:  "wrong predicate",
				input: []string{`to-add-predicate=true="Fo_o("123")"`},
			},
			{
				name:  "wrong predicate and empty value",
				input: []string{"", `to-add-predicate=true="Fo_o()"`},
			},
			{
				name:  "duplicate",
				input: []string{`to-add-predicate=true=Foo("123")`, `to-add-predicate=true=Bar("456")`},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := parseAnnotationPredicates(tc.input)
				assert.Error(t, err)
			})
		}
	})

	t.Run("parseAnnotationFilters", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			input []string
		}{
			{
				name:  "invalid filter",
				input: []string{`foo=bar=invalid-filter()`},
			},
			{
				name:  "invalid filter and empty value",
				input: []string{"", `foo=bar=invalid-filter()`},
			},
			{
				name:  "duplicate",
				input: []string{`foo=bar=baz("123")`, `foo=bar=qux("456")`},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := parseAnnotationFilters(tc.input)
				assert.Error(t, err)
			})
		}
	})
}

func TestParseAnnotationConfig(t *testing.T) {
	t.Run("parseAnnotationPredicates", func(t *testing.T) {

		for _, tc := range []struct {
			name     string
			input    []string
			expected []kubernetes.AnnotationPredicates
		}{
			{
				name:     "empty",
				input:    []string{},
				expected: nil,
			},
			{
				name:     "empty string",
				input:    []string{""},
				expected: nil,
			},
			{
				name: "empty string and a valid value",
				input: []string{
					"",
					`to-add-predicate=true=Foo("123")`,
				},
				expected: []kubernetes.AnnotationPredicates{
					{
						Key:        "to-add-predicate",
						Value:      "true",
						Predicates: eskip.MustParsePredicates(`Foo("123")`),
					},
				},
			},
			{
				name:  "single",
				input: []string{`to-add-predicate=true=Foo("123")`},
				expected: []kubernetes.AnnotationPredicates{
					{
						Key:        "to-add-predicate",
						Value:      "true",
						Predicates: eskip.MustParsePredicates(`Foo("123")`),
					},
				},
			},
			{
				name:  "multiple",
				input: []string{`to-add-predicate=true=Foo("123")`, `to-add-predicate=false=Bar("456") && Foo("789")`},
				expected: []kubernetes.AnnotationPredicates{
					{
						Key:        "to-add-predicate",
						Value:      "true",
						Predicates: eskip.MustParsePredicates(`Foo("123")`),
					},
					{
						Key:        "to-add-predicate",
						Value:      "false",
						Predicates: eskip.MustParsePredicates(`Bar("456") && Foo("789")`),
					},
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				val, err := parseAnnotationPredicates(tc.input)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, val)
			})
		}
	})

	t.Run("parseAnnotationFilters", func(t *testing.T) {

		for _, tc := range []struct {
			name     string
			input    []string
			expected []kubernetes.AnnotationFilters
		}{
			{
				name:     "empty",
				input:    []string{},
				expected: nil,
			},
			{
				name:     "empty string",
				input:    []string{""},
				expected: nil,
			},
			{
				name: "empty string and a valid value",
				input: []string{
					"",
					`foo=true=baz("123=456")`,
				},
				expected: []kubernetes.AnnotationFilters{
					{
						Key:     "foo",
						Value:   "true",
						Filters: eskip.MustParseFilters(`baz("123=456")`),
					},
				},
			},
			{
				name:  "single",
				input: []string{`foo=true=baz("123=456")`},
				expected: []kubernetes.AnnotationFilters{
					{
						Key:     "foo",
						Value:   "true",
						Filters: eskip.MustParseFilters(`baz("123=456")`),
					},
				},
			},
			{
				name:  "multiple",
				input: []string{`foo=true=baz("123=456")`, `foo=false=bar("456") -> foo("789")`},
				expected: []kubernetes.AnnotationFilters{
					{
						Key:     "foo",
						Value:   "true",
						Filters: eskip.MustParseFilters(`baz("123=456")`),
					},
					{
						Key:     "foo",
						Value:   "false",
						Filters: eskip.MustParseFilters(`bar("456") -> foo("789")`),
					},
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				val, err := parseAnnotationFilters(tc.input)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, val)
			})
		}
	})
}
