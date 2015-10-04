package run

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

// Options to start skipper. Expects address to listen on and one or more urls to find
// the etcd service at. If the flag 'insecure' is true, skipper will accept
// invalid TLS certificates from the backends.
type Options struct {
	Address                   string
	EtcdUrls                  []string
	StorageRoot               string
	Insecure                  bool
	InnkeeperUrl              string
	InnkeeperPollTimeout      time.Duration
	RoutesFilePath            string
	CustomFilters             []filters.Spec
	IgnoreTrailingSlash       bool
	OAuthUrl                  string
	OAuthScope                string
	InnkeeperAuthToken        string
	InnkeeperPreRouteFilters  []string
	InnkeeperPostRouteFilters []string
}

func createDataClient(o Options, auth innkeeper.Authentication) (routing.DataClient, error) {
	switch {
	case o.RoutesFilePath != "":
		return eskipfile.New(o.RoutesFilePath)
	case o.InnkeeperUrl != "":
		return innkeeper.New(innkeeper.Options{
			o.InnkeeperUrl, o.Insecure, o.InnkeeperPollTimeout, auth,
			o.InnkeeperPreRouteFilters, o.InnkeeperPostRouteFilters})
	default:
		return etcd.New(o.EtcdUrls, o.StorageRoot)
	}
}

func createInnkeeperAuthentication(o Options) innkeeper.Authentication {
	if o.InnkeeperAuthToken != "" {
		return innkeeper.FixedToken(o.InnkeeperAuthToken)
	}

	return oauth.New(o.OAuthUrl, o.OAuthScope)
}

// Run skipper.
//
// If a routesFilePath is given, that file will be used _instead_ of etcd.
func Run(o Options) error {
	// create authentication for Innkeeper
	auth := createInnkeeperAuthentication(o)

	// create data client
	dataClient, err := createDataClient(o, auth)
	if err != nil {
		return err
	}

	// create a filter registry with the available filter specs registered,
	// and register the custom filters
	registry := make(filters.Registry)
	for _, f := range o.CustomFilters {
		registry[f.Name()] = f
	}

	// create routing
	// create the proxy instance
	routing := routing.New(dataClient, registry, o.IgnoreTrailingSlash)
	proxy := proxy.New(routing, o.Insecure)

	// start the http server
	log.Printf("listening on %v\n", o.Address)
	return http.ListenAndServe(o.Address, proxy)
}
