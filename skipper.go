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
	"github.com/zalando/skipper/eskipfile"
	"github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/innkeeper"
	"github.com/zalando/skipper/oauth"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"log"
	"net/http"
	"time"
)

const (
	defaultSourcePollTimeout   = 30 * time.Millisecond
	defaultRoutingUpdateBuffer = 1 << 5
)

// Options to start skipper. Expects address to listen on and one or more urls to find
// the etcd service at. If the flag 'insecure' is true, skipper will accept
// invalid TLS certificates from the backends.
type Options struct {
	Address                   string
	EtcdUrls                  []string
	StorageRoot               string
	Insecure                  bool
	InnkeeperUrl              string
	SourcePollTimeout         time.Duration
	RoutesFile                string
	CustomFilters             []filters.Spec
	IgnoreTrailingSlash       bool
	OAuthUrl                  string
	OAuthCredentialsDir       string
	OAuthScope                string
	InnkeeperAuthToken        string
	InnkeeperPreRouteFilters  string
	InnkeeperPostRouteFilters string
	DevMode                   bool
}

func createDataClients(o Options, auth innkeeper.Authentication) ([]routing.DataClient, error) {
	var clients []routing.DataClient

	if o.RoutesFile != "" {
		f, err := eskipfile.Open(o.RoutesFile)
		if err != nil {
			return nil, err
		}

		clients = append(clients, f)
	}

	if o.InnkeeperUrl != "" {
		ic, err := innkeeper.New(innkeeper.Options{
			o.InnkeeperUrl, o.Insecure, auth,
			o.InnkeeperPreRouteFilters, o.InnkeeperPostRouteFilters})
		if err != nil {
			return nil, err
		}

		clients = append(clients, ic)
	}

	if len(o.EtcdUrls) > 0 {
		clients = append(clients, etcd.New(o.EtcdUrls, o.StorageRoot))
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
//
// If a routesFilePath is given, that file will be used _instead_ of etcd.
func Run(o Options) error {
	// create authentication for Innkeeper
	auth := createInnkeeperAuthentication(o)

	// create data client
	dataClients, err := createDataClients(o, auth)
	if err != nil {
		return err
	}

	if len(dataClients) == 0 {
		log.Println("warning: no route source specified")
	}

	// create a filter registry with the available filter specs registered,
	// and register the custom filters
	registry := filters.Defaults()
	for _, f := range o.CustomFilters {
		registry.Register(f)
	}

	// create routing
	// create the proxy instance
	var mo routing.MatchingOptions
	if o.IgnoreTrailingSlash {
		mo = routing.IgnoreTrailingSlash
	}

	if o.SourcePollTimeout <= 0 {
		o.SourcePollTimeout = defaultSourcePollTimeout
	}

	updateBuffer := defaultRoutingUpdateBuffer
	if o.DevMode {
		updateBuffer = 0
	}

	routing := routing.New(routing.Options{
		registry,
		mo,
		o.SourcePollTimeout,
		dataClients,
		updateBuffer})
	proxy := proxy.New(routing, o.Insecure)

	// start the http server
	log.Printf("listening on %v\n", o.Address)
	return http.ListenAndServe(o.Address, proxy)
}
