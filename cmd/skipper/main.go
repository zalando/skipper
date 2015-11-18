// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
This command provides an executable version of skipper with the default
set of filters.

For the list of command line options run:

    skipper -help

For details about the usage and extensibility of skipper, please see the
documentation of the root skipper package.

To see which built-in filters are available, see the skipper/filters
package documentation.
*/
package main

import (
	"flag"
	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/proxy"
	"strings"
	"time"
)

const (
	defaultAddress           = ":9090"
	defaultEtcdPrefix        = "/skipper"
	defaultSourcePollTimeout = int64(3000)
	defaultMetricsListener   = ":9911"
	defaultMetricsPrefix     = "skipper."

	addressUsage                   = "network address that skipper should listen on"
	etcdUrlsUsage                  = "urls of nodes in an etcd cluster, storing route definitions"
	etcdPrefixUsage                = "path prefix for skipper related data in etcd"
	innkeeperUrlUsage              = "API endpoint of the Innkeeper service, storing route definitions"
	innkeeperAuthTokenUsage        = "fixed token for innkeeper authentication"
	innkeeperPreRouteFiltersUsage  = "filters to be prepended to each route loaded from Innkeeper"
	innkeeperPostRouteFiltersUsage = "filters to be appended to each route loaded from Innkeeper"
	oauthUrlUsage                  = "OAuth2 URL for Innkeeper authentication"
	oauthCredentialsDirUsage       = "directory where oauth credentials are stored: client.json and user.json"
	oauthScopeUsage                = "the whitespace separated list of oauth scopes"
	routesFileUsage                = "file containing static route definitions"
	sourcePollTimeoutUsage         = "polling timeout of the routing data sources, in milliseconds"
	insecureUsage                  = "flag indicating to ignore the verification of the TLS certificates of the backend services"
	devModeUsage                   = "enables developer time behavior, like ubuffered routing updates"
	metricsListenerUsage           = "network address used for exposing the /metrics endpoint. An empty value disables metrics."
	metricsPrefixUsage             = "allows setting a custom path prefix for metrics export"
	debugGcMetricsUsage            = "enables reporting of the Go garbage collector statistics exported in debug.GCStats"
	runtimeMetricsUsage            = "enables reporting of the Go runtime statistics exported in runtime and specifically runtime.MemStats"
	applicationLogUsage            = "output file for the application log. When not set, /dev/stderr is used"
	applicationLogPrefixUsage      = "prefix for each log entry"
	accessLogUsage                 = "output file for the access log, When not set, /dev/stderr is used"
	accessLogDisabledUsage         = "when this flag is set, no access log is printed"
)

var (
	address                   string
	etcdUrls                  string
	etcdPrefix                string
	insecure                  bool
	innkeeperUrl              string
	sourcePollTimeout         int64
	routesFile                string
	oauthUrl                  string
	oauthScope                string
	oauthCredentialsDir       string
	innkeeperAuthToken        string
	innkeeperPreRouteFilters  string
	innkeeperPostRouteFilters string
	devMode                   bool
	metricsListener           string
	metricsPrefix             string
	debugGcMetrics            bool
	runtimeMetrics            bool
	applicationLog            string
	applicationLogPrefix      string
	accessLog                 string
	accessLogDisabled         bool
)

func init() {
	flag.StringVar(&address, "address", defaultAddress, addressUsage)
	flag.StringVar(&etcdUrls, "etcd-urls", "", etcdUrlsUsage)
	flag.BoolVar(&insecure, "insecure", false, insecureUsage)
	flag.StringVar(&etcdPrefix, "etcd-prefix", defaultEtcdPrefix, etcdPrefixUsage)
	flag.StringVar(&innkeeperUrl, "innkeeper-url", "", innkeeperUrlUsage)
	flag.Int64Var(&sourcePollTimeout, "source-poll-timeout", defaultSourcePollTimeout, sourcePollTimeoutUsage)
	flag.StringVar(&routesFile, "routes-file", "", routesFileUsage)
	flag.StringVar(&oauthUrl, "oauth-url", "", oauthUrlUsage)
	flag.StringVar(&oauthScope, "oauth-scope", "", oauthScopeUsage)
	flag.StringVar(&oauthCredentialsDir, "oauth-credentials-dir", "", oauthCredentialsDirUsage)
	flag.StringVar(&innkeeperAuthToken, "innkeeper-auth-token", "", innkeeperAuthTokenUsage)
	flag.StringVar(&innkeeperPreRouteFilters, "innkeeper-pre-route-filters", "", innkeeperPreRouteFiltersUsage)
	flag.StringVar(&innkeeperPostRouteFilters, "innkeeper-post-route-filters", "", innkeeperPostRouteFiltersUsage)
	flag.BoolVar(&devMode, "dev-mode", false, devModeUsage)
	flag.StringVar(&metricsListener, "metrics-listener", defaultMetricsListener, metricsListenerUsage)
	flag.StringVar(&metricsPrefix, "metrics-prefix", defaultMetricsPrefix, metricsPrefixUsage)
	flag.BoolVar(&debugGcMetrics, "debug-gc-metrics", false, debugGcMetricsUsage)
	flag.BoolVar(&runtimeMetrics, "runtime-metrics", true, runtimeMetricsUsage)
	flag.StringVar(&applicationLog, "application-log", "", applicationLogUsage)
	flag.StringVar(&applicationLogPrefix, "application-log-prefix", "", applicationLogPrefixUsage)
	flag.StringVar(&accessLog, "access-log", "", accessLogUsage)
	flag.BoolVar(&accessLogDisabled, "access-log-disabled", false, accessLogDisabledUsage)
	flag.Parse()
}

func main() {
	var eus []string
	if len(etcdUrls) > 0 {
		eus = strings.Split(etcdUrls, ",")
	}

	options := skipper.Options{
		Address:                   address,
		EtcdUrls:                  eus,
		EtcdPrefix:                etcdPrefix,
		InnkeeperUrl:              innkeeperUrl,
		SourcePollTimeout:         time.Duration(sourcePollTimeout) * time.Millisecond,
		RoutesFile:                routesFile,
		IgnoreTrailingSlash:       false,
		OAuthUrl:                  oauthUrl,
		OAuthScope:                oauthScope,
		OAuthCredentialsDir:       oauthCredentialsDir,
		InnkeeperAuthToken:        innkeeperAuthToken,
		InnkeeperPreRouteFilters:  innkeeperPreRouteFilters,
		InnkeeperPostRouteFilters: innkeeperPostRouteFilters,
		DevMode:                   devMode,
		MetricsListener:           metricsListener,
		MetricsPrefix:             metricsPrefix,
		EnableDebugGcMetrics:      debugGcMetrics,
		EnableRuntimeMetrics:      runtimeMetrics,
		ApplicationLogOutput:      applicationLog,
		ApplicationLogPrefix:      applicationLogPrefix,
		AccessLogOutput:           accessLog,
		AccessLogDisabled:         accessLogDisabled}
	if insecure {
		options.ProxyOptions |= proxy.OptionsInsecure
	}

	log.Fatal(skipper.Run(options))
}
