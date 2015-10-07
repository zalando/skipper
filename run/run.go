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

const defaultSourcePollTimeout = 30 * time.Millisecond

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
	RoutesFilePath            string
	CustomFilters             []filters.Spec
	IgnoreTrailingSlash       bool
	OAuthUrl                  string
	OAuthScope                string
	InnkeeperAuthToken        string
	InnkeeperPreRouteFilters  string
	InnkeeperPostRouteFilters string
}

func createDataClients(o Options, auth innkeeper.Authentication) ([]routing.DataClient, error) {
	var clients []routing.DataClient
	switch {
	case o.RoutesFilePath != "":
		clients = append(clients, eskipfile.Client(o.RoutesFilePath))
	case o.InnkeeperUrl != "":
		ic, err := innkeeper.New(innkeeper.Options{
			o.InnkeeperUrl, o.Insecure, auth,
			o.InnkeeperPreRouteFilters, o.InnkeeperPostRouteFilters})
		if err != nil {
			return nil, err
		}

		clients = append(clients, ic)
	default:
		clients = append(clients, etcd.New(o.EtcdUrls, o.StorageRoot))
	}

	return clients, nil
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
	dataClients, err := createDataClients(o, auth)
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
	var mo routing.MatchingOptions
	if o.IgnoreTrailingSlash {
		mo = routing.IgnoreTrailingSlash
	}

	if o.SourcePollTimeout <= 0 {
		o.SourcePollTimeout = defaultSourcePollTimeout
	}

	routing := routing.New(routing.Options{registry, mo, o.SourcePollTimeout, dataClients})
	proxy := proxy.New(routing, o.Insecure)

	// start the http server
	log.Printf("listening on %v\n", o.Address)
	return http.ListenAndServe(o.Address, proxy)
}
