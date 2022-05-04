package config

import (
	"crypto/tls"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/google/go-cmp/cmp"
)

// Move configuration init here to avoid race conditions when parsing flags in multiple tests
var cfg = NewConfig()

func TestEnvOverrides_SwarmRedisPassword(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

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
			// Run test in non-concurrent way to avoid collisions of other test cases that parse config file
			os.Args = tt.args
			if tt.env != "" {
				t.Setenv(redisPasswordEnv, tt.env)
			}
			err := cfg.Parse()
			if err != nil {
				t.Errorf("config.NewConfig() error = %v", err)
			}

			if cfg.SwarmRedisPassword != tt.want {
				t.Errorf("cfg.SwarmRedisPassword didn't set correctly: Want '%s', got '%s'", tt.want, cfg.SwarmRedisPassword)
			}
		})
	}
}

func Test_NewConfig(t *testing.T) {
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
			want: &Config{
				ConfigFile:                              "testdata/test.yaml",
				Address:                                 "localhost:8080",
				StatusChecks:                            nil,
				ExpectedBytesPerRequest:                 50 * 1024,
				SupportListener:                         ":9911",
				MaxLoopbacks:                            12,
				DefaultHTTPStatus:                       404,
				MaxAuditBody:                            1024,
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
				EtcdTimeout:                             2 * time.Second,
				AppendFilters:                           &defaultFiltersFlags{},
				PrependFilters:                          &defaultFiltersFlags{},
				CloneRoute:                              &routeChangerConfig{},
				EditRoute:                               &routeChangerConfig{},
				SourcePollTimeout:                       3000,
				KubernetesEastWestRangeDomains:          commaListFlag(),
				KubernetesHealthcheck:                   true,
				KubernetesHTTPSRedirect:                 true,
				KubernetesHTTPSRedirectCode:             308,
				KubernetesPathModeString:                "kubernetes-ingress",
				Oauth2TokeninfoTimeout:                  2 * time.Second,
				Oauth2TokenintrospectionTimeout:         2 * time.Second,
				Oauth2TokeninfoSubjectKey:               "uid",
				Oauth2TokenCookieName:                   "oauth2-grant",
				WebhookTimeout:                          2 * time.Second,
				OidcDistributedClaimsTimeout:            2 * time.Second,
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
				SwarmRedisPassword:                      "set_from_file",
				SwarmRedisDialTimeout:                   25 * time.Millisecond,
				SwarmRedisReadTimeout:                   25 * time.Millisecond,
				SwarmRedisWriteTimeout:                  25 * time.Millisecond,
				SwarmRedisPoolTimeout:                   25 * time.Millisecond,
				SwarmRedisMinConns:                      100,
				SwarmRedisMaxConns:                      100,
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
				RefusePayload:                           multiFlag{"foo", "bar", "baz"},
			},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			os.Args = tt.args

			err := cfg.Parse()
			if (err != nil) != tt.wantErr {
				t.Errorf("config.NewConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if cmp.Equal(cfg, tt.want, cmp.AllowUnexported(listFlag{}, pluginFlag{}, defaultFiltersFlags{}, mapFlags{})) == false {
					t.Errorf("config.NewConfig() got vs. want:\n%v", cmp.Diff(cfg, tt.want, cmp.AllowUnexported(listFlag{}, pluginFlag{}, defaultFiltersFlags{}, mapFlags{})))
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

type testFormatter struct {
	messages map[string]log.Level
}

func (tf *testFormatter) Format(entry *log.Entry) ([]byte, error) {
	tf.messages[entry.Message] = entry.Level
	return nil, nil
}

func TestDeprecatedFlags(t *testing.T) {
	a := os.Args
	o := log.StandardLogger().Out
	f := log.StandardLogger().Formatter
	defer func() {
		os.Args = a
		log.SetOutput(o)
		log.SetFormatter(f)
	}()

	formatter := &testFormatter{messages: make(map[string]log.Level)}

	os.Args = []string{
		"skipper",
		"-config-file=testdata/deprecated.yaml",
		"-enable-prometheus-metrics=false", // default value should produce deprecation warning as well
		"-api-usage-monitoring-default-client-tracking-pattern=whatever",
	}
	log.SetOutput(os.Stdout)
	log.SetFormatter(formatter)

	err := cfg.Parse()
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
