package main

import (
	"fmt"
	"log"
	"net/http"
	"skipper/dispatch"
	"skipper/etcd"
	"skipper/middleware"
	"skipper/proxy"
	"skipper/settings"
	"skipper/skipper"
	"time"
)

const startupSettingsTimeout = 1200 * time.Millisecond

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
	dataClient, err := etcd.Make([]string{"http://127.0.0.1:2379", "http://127.0.0.1:4001"}, "/skipper")
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
	settings := waitForInitialSettings(settingsChan)

	// todo: print only in dev environment
	//
	// note:
	// address here is only for testing that there can be other settings
	// in etcd and processed by settings, but the address itself actually
	// best if comes the flags
	address := settings.Address()
	fmt.Printf("listening on %v\n", address)

	log.Fatal(http.ListenAndServe(address, proxy))
}
