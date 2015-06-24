// skipper program main
//
// command line flags:
//
// -etcd-urls:
// one or more urls where the etcd service to be used can be found
//
// -insecure:
// if set, skipper accepts invalid certificates from backend hosts
//
// (see the skipper package for an overview of the program structure)
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"skipper/dispatch"
	"skipper/etcd"
	"skipper/middleware"
	"skipper/proxy"
	"skipper/settings"
	"skipper/skipper"
	"strings"
	"time"
)

const (
	startupSettingsTimeout = 1200 * time.Millisecond
	storageRoot            = "/skipper"
	defaultAddress         = ":9090"
	defaultEtcdUrls        = "http://127.0.0.1:2379,http://127.0.0.1:4001"
	addressUsage           = "address where skipper should listen on"
	etcdUrlsUsage          = "urls where etcd can be found"
	insecureUsage          = "set this flag to allow invalid certificates for tls connections"
)

var (
	address  string
	etcdUrls string
	insecure bool
)

func init() {
	flag.StringVar(&address, "address", defaultAddress, addressUsage)
	flag.StringVar(&etcdUrls, "etcd-urls", defaultEtcdUrls, etcdUrlsUsage)
	flag.BoolVar(&insecure, "insecure", false, insecureUsage)
	flag.Parse()
}

func waitForInitialSettings(c <-chan skipper.Settings) skipper.Settings {
	// todo:
	//  not good, because due to the fan, it is basically a busy loop
	//  maybe it just shouldn't let nil through
	for {
		time.Sleep(12 * time.Millisecond)

		select {
		case s := <-c:
			if s != nil {
				return s
			}
		case <-time.After(startupSettingsTimeout):
			log.Fatal("initial settings timeout")
		}
	}
}

func main() {
	// create a client to etcd
	dataClient, err := etcd.Make(strings.Split(etcdUrls, ","), storageRoot)
	if err != nil {
		log.Fatal(err)
		return
	}

	// create a middleware registry with the available middleware registered
	// create a settings dispatcher instance
	// create a settings source
	// create the proxy instance
	registry := middleware.RegisterDefault()
	dispatcher := dispatch.Make()
	settingsSource := settings.MakeSource(dataClient, registry, dispatcher)
	proxy := proxy.Make(settingsSource, insecure)

	// subscribe to new settings
	settingsChan := make(chan skipper.Settings)
	dispatcher.Subscribe(settingsChan)

	// don't start the server until the initial settings have been received
	// todo: exit if this fails(?)
	waitForInitialSettings(settingsChan)

	// be a little polite
	// todo: print only in dev environment
	fmt.Printf("listening on %v\n", address)

	// start the http server
	log.Fatal(http.ListenAndServe(address, proxy))
}
