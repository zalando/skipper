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
	"github.com/golang/glog"
	"github.com/rcrowley/go-metrics"
	"github.com/zalando/skipper/eskipfile"
	"github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/innkeeper"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/oauth"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"net/http"
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

	// API endpoint of the Innkeeper service, storing route definitions.
	InnkeeperUrl string

	// Fixed token for innkeeper authentication. (Used mainly in
	// development environments.)
	InnkeeperAuthToken string

	// Filters to be prepended to each route loaded from Innkeeper.
	InnkeeperPreRouteFilters string

	// Filters to be appended to each route loaded from Innkeeper.
	InnkeeperPostRouteFilters string

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

	// Dev mode. Currently this flag disables prioritization of the
	// consumer side over the feeding side during the routing updates to
	// populate the updated routes faster.
	DevMode bool
}

func createDataClients(o Options, auth innkeeper.Authentication) ([]routing.DataClient, error) {
	var clients []routing.DataClient

	if o.RoutesFile != "" {
		f, err := eskipfile.Open(o.RoutesFile)
		if err != nil {
			glog.Error(err)
			return nil, err
		}

		clients = append(clients, f)
	}

	if o.InnkeeperUrl != "" {
		ic, err := innkeeper.New(innkeeper.Options{
			o.InnkeeperUrl, o.ProxyOptions.Insecure(), auth,
			o.InnkeeperPreRouteFilters, o.InnkeeperPostRouteFilters})
		if err != nil {
			glog.Error(err)
			return nil, err
		}

		clients = append(clients, ic)
	}

	if len(o.EtcdUrls) > 0 {
		clients = append(clients, etcd.New(o.EtcdUrls, o.EtcdPrefix))
	}

	return clients, nil
}

func createInnkeeperAuthentication(o Options) innkeeper.Authentication {
	if o.InnkeeperAuthToken != "" {
		return innkeeper.FixedToken(o.InnkeeperAuthToken)
	}

	return oauth.New(o.OAuthCredentialsDir, o.OAuthUrl, o.OAuthScope)
}

// Run skipper.
func Run(o Options) error {
	// create authentication for Innkeeper
	auth := createInnkeeperAuthentication(o)

	// create data client
	dataClients, err := createDataClients(o, auth)
	if err != nil {
		return err
	}

	if len(dataClients) == 0 {
		glog.Warning("no route source specified")
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

	// create a routing engine
	routing := routing.New(routing.Options{
		registry,
		mo,
		o.SourcePollTimeout,
		dataClients,
		updateBuffer})

	r := metrics.NewRegistry()

	// Have some options enabling these?
	metrics.RegisterDebugGCStats(r)
	go metrics.CaptureDebugGCStats(r, 5e9)

	metrics.RegisterRuntimeMemStats(r)
	go metrics.CaptureRuntimeMemStats(r, 5e9)

	// create the proxy
	proxy := proxy.New(routing, o.ProxyOptions, r, o.PriorityRoutes...)

	// create the access log handler
	loggingHandler := logging.NewHandler(proxy, r)

	// start the http server
	glog.Infof("listening on %v\n", o.Address)
	return http.ListenAndServe(o.Address, loggingHandler)
}
