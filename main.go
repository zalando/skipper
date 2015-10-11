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
package main

import (
	"flag"
	"github.com/zalando/skipper/run"
	"log"
	"strings"
	"time"
)

const (
	defaultAddress           = ":9090"
	defaultEtcdUrls          = "http://127.0.0.1:2379,http://127.0.0.1:4001"
	defaultStorageRoot       = "/skipper"
	defaultSourcePollTimeout = int64(180 * time.Second)

	addressUsage                   = "address where skipper should listen on"
	etcdUrlsUsage                  = "urls where etcd can be found"
	insecureUsage                  = "set this flag to allow invalid certificates for tls connections"
	storageRootUsage               = "prefix for skipper related data in the provided etcd storage"
	innkeeperUrlUsage              = "url of the innkeeper API"
	sourcePollTimeoutUsage         = "polling timeout of the routing data sources"
	oauthUrlUsage                  = "OAuth2 URL for Innkeeper authentication"
	routesFileUsage                = "routes file to use instead of etcd"
	innkeeperAuthTokenUsage        = "fixed token for innkeeper authentication"
	innkeeperPreRouteFiltersUsage  = "global pre-route filters for routes from Innkeeper"
	innkeeperPostRouteFiltersUsage = "global post-route filters for routes from Innkeeper"
	devModeUsage                   = "enables developer time behavior, like ubuffered routing updates"
)

var (
	address                   string
	etcdUrls                  string
	insecure                  bool
	storageRoot               string
	innkeeperUrl              string
	sourcePollTimeout         int64
	routesFile                string
	oauthUrl                  string
	innkeeperAuthToken        string
	innkeeperPreRouteFilters  string
	innkeeperPostRouteFilters string
	devMode                   bool
)

func init() {
	flag.StringVar(&address, "address", defaultAddress, addressUsage)
	flag.StringVar(&etcdUrls, "etcd-urls", defaultEtcdUrls, etcdUrlsUsage)
	flag.BoolVar(&insecure, "insecure", false, insecureUsage)
	flag.StringVar(&storageRoot, "storage-root", defaultStorageRoot, storageRootUsage)
	flag.StringVar(&innkeeperUrl, "innkeeper-url", "", innkeeperUrlUsage)
	flag.Int64Var(&sourcePollTimeout, "source-poll-timeout", defaultSourcePollTimeout, sourcePollTimeoutUsage)
	flag.StringVar(&routesFile, "routes-file", "", routesFileUsage)
	flag.StringVar(&oauthUrl, "oauth-url", "", oauthUrlUsage)
	flag.StringVar(&innkeeperAuthToken, "innkeeper-auth-token", "", innkeeperAuthTokenUsage)
	flag.StringVar(&innkeeperPreRouteFilters, "innkeeper-pre-route-filters", "", innkeeperPreRouteFiltersUsage)
	flag.StringVar(&innkeeperPostRouteFilters, "innkeeper-post-route-filters", "", innkeeperPostRouteFiltersUsage)
	flag.BoolVar(&devMode, "dev-mode", false, devModeUsage)
	flag.Parse()
}

func main() {
	log.Fatal(run.Run(run.Options{
		Address:                   address,
		EtcdUrls:                  strings.Split(etcdUrls, ","),
		StorageRoot:               storageRoot,
		Insecure:                  insecure,
		InnkeeperUrl:              innkeeperUrl,
		SourcePollTimeout:         time.Duration(sourcePollTimeout),
		RoutesFile:                routesFile,
		IgnoreTrailingSlash:       false,
		OAuthUrl:                  oauthUrl,
		InnkeeperAuthToken:        innkeeperAuthToken,
		InnkeeperPreRouteFilters:  innkeeperPreRouteFilters,
		InnkeeperPostRouteFilters: innkeeperPostRouteFilters,
		DevMode:                   devMode}))
}
