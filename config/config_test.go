package config

import (
	"os"
	"reflect"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/google/go-cmp/cmp"
)

func Test_NewConfig(t *testing.T) {
	cfg := NewConfig()

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
			args: []string{"skipper", "-config-file=test.yaml", "-address=localhost:8080"},
			want: &Config{
				ConfigFile:                              "test.yaml",
				Address:                                 "localhost:8080",
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
				SourcePollTimeout:                       3000,
				KubernetesHealthcheck:                   true,
				KubernetesHTTPSRedirect:                 true,
				KubernetesHTTPSRedirectCode:             308,
				KubernetesPathModeString:                "kubernetes-ingress",
				Oauth2TokeninfoTimeout:                  2 * time.Second,
				Oauth2TokenintrospectionTimeout:         2 * time.Second,
				WebhookTimeout:                          2 * time.Second,
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
				SwarmRedisURLs:                          commaListFlag(),
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
				if cmp.Equal(cfg, tt.want, cmp.AllowUnexported(listFlag{}, pluginFlag{}, defaultFiltersFlags{})) == false {
					t.Errorf("config.NewConfig() got vs. want:\n%v", cmp.Diff(cfg, tt.want, cmp.AllowUnexported(listFlag{}, pluginFlag{}, defaultFiltersFlags{})))
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
