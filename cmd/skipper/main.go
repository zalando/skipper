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

// skipper program main
//
// for a summary about skipper, please see the readme file.

/*
Command skipper provides a default executable skipper with the default
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
	"github.com/zalando/skipper"
	"log"
	"strings"
	"time"
)

const (
	defaultAddress           = ":9090"
	defaultEtcdUrls          = "http://127.0.0.1:2379,http://127.0.0.1:4001"
	defaultEtcdStorageRoot   = "/skipper"
	defaultSourcePollTimeout = int64(3000)

	addressUsage                   = "network address that skipper should listen on"
	etcdUrlsUsage                  = "urls of nodes in an etcd cluster, storing route definitions"
	etcdStorageRootUsage           = "path prefix for skipper related data in the etcd storage"
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
)

var (
	address                   string
	etcdUrls                  string
	etcdStorageRoot           string
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
)

func init() {
	flag.StringVar(&address, "address", defaultAddress, addressUsage)
	flag.StringVar(&etcdUrls, "etcd-urls", defaultEtcdUrls, etcdUrlsUsage)
	flag.BoolVar(&insecure, "insecure", false, insecureUsage)
	flag.StringVar(&etcdStorageRoot, "storage-root", defaultEtcdStorageRoot, etcdStorageRootUsage)
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
	flag.Parse()
}

func main() {
	log.Fatal(skipper.Run(skipper.Options{
		Address:                   address,
		EtcdUrls:                  strings.Split(etcdUrls, ","),
		EtcdStorageRoot:           etcdStorageRoot,
		Insecure:                  insecure,
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
		DevMode:                   devMode}))
}
