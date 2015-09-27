package run

import (
	"github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy"
    "github.com/zalando/skipper/eskipfile"
    "github.com/zalando/skipper/routing"
	"log"
	"net/http"
)

// Run skipper. Expects address to listen on and one or more urls to find
// the etcd service at. If the flag 'insecure' is true, skipper will accept
// invalid TLS certificates from the backends.
// If a routesFilePath is given, that file will be used INSTEAD of etcd
func Run(address string, etcdUrls []string, storageRoot string, insecure bool, routesFilePath string,
	ignoreTrailingSlash bool, customFilters ...filters.Spec) error {

	var dataClient routing.DataClient
	var err error
	if len(routesFilePath) > 0 {
		dataClient, err = eskipfile.Make(routesFilePath)
		if err != nil {
			return err
		}
	} else {
		dataClient, err = etcd.Make(etcdUrls, storageRoot)
		if err != nil {
			return err
		}
	}

	// create a filter registry with the available filter specs registered,
	// and register the custom filters
	registry := filters.DefaultFilters()
	for _, f := range customFilters {
		registry[f.Name()] = f
	}

	// create routing
	// create the proxy instance
    routing := routing.New(dataClient, registry, ignoreTrailingSlash)
	proxy := proxy.Make(routing, insecure)

	// start the http server
	log.Printf("listening on %v\n", address)
	return http.ListenAndServe(address, proxy)
}
