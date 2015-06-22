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
)

var (
	address  string
	etcdUrls string
)

func init() {
	flag.StringVar(&address, "address", defaultAddress, addressUsage)
	flag.StringVar(&etcdUrls, "etcd-urls", defaultEtcdUrls, etcdUrlsUsage)
	flag.Parse()
}

func waitForInitialSettings(c <-chan skipper.Settings) skipper.Settings {
	// todo:
	//  not good, because due to the fan, it is basically a busy loop
	//  maybe it just shouldn't let nil through
	for {
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
	dataClient, err := etcd.Make(strings.Split(etcdUrls, ","), storageRoot)
	if err != nil {
		log.Fatal(err)
		return
	}

	registry := middleware.RegisterDefault()
	dispatcher := dispatch.Make()
	settingsSource := settings.MakeSource(dataClient, registry, dispatcher)
	proxy := proxy.Make(settingsSource)

	settingsChan := make(chan skipper.Settings)
	dispatcher.Subscribe(settingsChan)

	// todo: exit if this fails(?)
	waitForInitialSettings(settingsChan)

	// todo: print only in dev environment
	fmt.Printf("listening on %v\n", address)

	log.Fatal(http.ListenAndServe(address, proxy))
}
