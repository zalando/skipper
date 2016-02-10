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

package skipper

import (
	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskipfile"
	"github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/innkeeper"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/predicates/source"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"io"
	"net/http"
	"os"
	"path"
	"time"
)

const (
	defaultSourcePollTimeout   = 30 * time.Millisecond
	defaultRoutingUpdateBuffer = 1 << 5
)

// Options to start skipper.
type Options struct {

	// Network address that skipper should listen on.
	Address string

	// List of custom filter specifications.
	CustomFilters []filters.Spec

	// Urls of nodes in an etcd cluster, storing route definitions.
	EtcdUrls []string

	// Path prefix for skipper related data in the etcd storage.
	EtcdPrefix string

	// Timeout used for a single request when querying for updates
	// in etcd. This is independent of, and an addition to,
	// SourcePollTimeout. When not set, the internally defined 1s
	// is used.
	EtcdWaitTimeout time.Duration

	// Skip TLS certificate check for etcd connections.
	EtcdInsecure bool

	// API endpoint of the Innkeeper service, storing route definitions.
	InnkeeperUrl string

	// Fixed token for innkeeper authentication. (Used mainly in
	// development environments.)
	InnkeeperAuthToken string

	// Filters to be prepended to each route loaded from Innkeeper.
	InnkeeperPreRouteFilters string

	// Filters to be appended to each route loaded from Innkeeper.
	InnkeeperPostRouteFilters string

	// Skip TLS certificate check for Innkeeper connections.
	InnkeeperInsecure bool

	// OAuth2 URL for Innkeeper authentication.
	OAuthUrl string

	// Directory where oauth credentials are stored, with file names:
	// client.json and user.json.
	OAuthCredentialsDir string

	// The whitespace separated list of OAuth2 scopes.
	OAuthScope string

	// File containing static route definitions.
	RoutesFile string

	// Polling timeout of the routing data sources.
	SourcePollTimeout time.Duration

	// Flags controlling the proxy behavior.
	ProxyOptions proxy.Options

	// Flag indicating to ignore trailing slashes in paths during route
	// lookup.
	IgnoreTrailingSlash bool

	// Priority routes that are matched against the requests before
	// the standard routes from the data clients.
	PriorityRoutes []proxy.PriorityRoute

	// Specifications of custom, user defined predicates.
	CustomPredicates []routing.PredicateSpec

	// Dev mode. Currently this flag disables prioritization of the
	// consumer side over the feeding side during the routing updates to
	// populate the updated routes faster.
	DevMode bool

	// Network address for the /metrics endpoint
	MetricsListener string

	// Skipper provides a set of metrics with different keys which are exposed via HTTP in JSON
	// You can customize those key names with your own prefix
	MetricsPrefix string

	// Flag that enables reporting of the Go garbage collector statistics exported in debug.GCStats
	EnableDebugGcMetrics bool

	// Flag that enables reporting of the Go runtime statistics exported in runtime and specifically runtime.MemStats
	EnableRuntimeMetrics bool

	// Output file for the application log. Default value: /dev/stderr.
	//
	// When /dev/stderr or /dev/stdout is passed in, it will be resolved
	// to os.Stderr or os.Stdout.
	//
	// Warning: passing an arbitrary file will try to open it append
	// on start and use it, or fail on start, but the current
	// implementation doesn't support any more proper handling
	// of temporary failures or log-rolling.
	ApplicationLogOutput string

	// Application log prefix. Default value: "[APP]".
	ApplicationLogPrefix string

	// Output file for the access log. Default value: /dev/stderr.
	//
	// When /dev/stderr or /dev/stdout is passed in, it will be resolved
	// to os.Stderr or os.Stdout.
	//
	// Warning: passing an arbitrary file will try to open for append
	// it on start and use it, or fail on start, but the current
	// implementation doesn't support any more proper handling
	// of temporary failures or log-rolling.
	AccessLogOutput string

	// Disables the access log.
	AccessLogDisabled bool
}

func createDataClients(o Options, auth innkeeper.Authentication) ([]routing.DataClient, error) {
	var clients []routing.DataClient

	if o.RoutesFile != "" {
		f, err := eskipfile.Open(o.RoutesFile)
		if err != nil {
			log.Error(err)
			return nil, err
		}

		clients = append(clients, f)
	}

	if o.InnkeeperUrl != "" {
		ic, err := innkeeper.New(innkeeper.Options{
			o.InnkeeperUrl, o.InnkeeperInsecure, auth,
			o.InnkeeperPreRouteFilters, o.InnkeeperPostRouteFilters})
		if err != nil {
			log.Error(err)
			return nil, err
		}

		clients = append(clients, ic)
	}

	if len(o.EtcdUrls) > 0 {
		etcdClient, err := etcd.New(etcd.Options{
			o.EtcdUrls,
			o.EtcdPrefix,
			o.EtcdWaitTimeout,
			o.EtcdInsecure})
		if err != nil {
			return nil, err
		}

		clients = append(clients, etcdClient)
	}

	return clients, nil
}

func getLogOutput(name string) (io.Writer, error) {
	name = path.Clean(name)

	if name == "/dev/stdout" {
		return os.Stdout, nil
	}

	if name == "/dev/stderr" {
		return os.Stderr, nil
	}

	return os.OpenFile(name, os.O_APPEND, os.ModeAppend)
}

func initLog(o Options) error {
	var (
		logOutput       io.Writer
		accessLogOutput io.Writer
		err             error
	)

	if o.ApplicationLogOutput != "" {
		logOutput, err = getLogOutput(o.ApplicationLogOutput)
		if err != nil {
			return err
		}
	}

	if !o.AccessLogDisabled && o.AccessLogOutput != "" {
		accessLogOutput, err = getLogOutput(o.AccessLogOutput)
		if err != nil {
			return err
		}
	}

	logging.Init(logging.Options{
		ApplicationLogPrefix: o.ApplicationLogPrefix,
		ApplicationLogOutput: logOutput,
		AccessLogOutput:      accessLogOutput,
		AccessLogDisabled:    o.AccessLogDisabled})

	return nil
}

// Run skipper.
func Run(o Options) error {
	// init log
	err := initLog(o)
	if err != nil {
		return err
	}

	// init metrics
	metrics.Init(metrics.Options{
		Listener:             o.MetricsListener,
		Prefix:               o.MetricsPrefix,
		EnableDebugGcMetrics: o.EnableDebugGcMetrics,
		EnableRuntimeMetrics: o.EnableRuntimeMetrics,
	})

	// create authentication for Innkeeper
	auth := innkeeper.CreateInnkeeperAuthentication(innkeeper.AuthOptions{
		InnkeeperAuthToken:  o.InnkeeperAuthToken,
		OAuthCredentialsDir: o.OAuthCredentialsDir,
		OAuthUrl:            o.OAuthUrl,
		OAuthScope:          o.OAuthScope})

	// create data client
	dataClients, err := createDataClients(o, auth)
	if err != nil {
		return err
	}

	if len(dataClients) == 0 {
		log.Warning("no route source specified")
	}

	// create a filter registry with the available filter specs registered,
	// and register the custom filters
	registry := builtin.MakeRegistry()
	for _, f := range o.CustomFilters {
		registry.Register(f)
	}

	// create routing
	// create the proxy instance
	var mo routing.MatchingOptions
	if o.IgnoreTrailingSlash {
		mo = routing.IgnoreTrailingSlash
	}

	// ensure a non-zero poll timeout
	if o.SourcePollTimeout <= 0 {
		o.SourcePollTimeout = defaultSourcePollTimeout
	}

	// check for dev mode, and set update buffer of the routes
	updateBuffer := defaultRoutingUpdateBuffer
	if o.DevMode {
		updateBuffer = 0
	}

	// include bundeled custom predicates
	o.CustomPredicates = append(o.CustomPredicates, source.New())

	// create a routing engine
	routing := routing.New(routing.Options{
		registry,
		mo,
		o.SourcePollTimeout,
		dataClients,
		o.CustomPredicates,
		updateBuffer})

	// create the proxy
	proxy := proxy.New(routing, o.ProxyOptions, o.PriorityRoutes...)

	// create the access log handler
	loggingHandler := logging.NewHandler(proxy)

	// start the http server
	log.Infof("proxy listener on %v", o.Address)
	return http.ListenAndServe(o.Address, loggingHandler)
}
