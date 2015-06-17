package main

import (
	"fmt"
	"log"
	"net/http"
	"skipper/dispatch"
	"skipper/mock"
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
	mockDataClient := mock.MakeDataClient(map[string]interface{}{
		"backends": map[string]interface{}{"hello": "http://localhost:9999/slow"},
		"frontends": []interface{}{
			map[string]interface{}{
				"route":      "Path(\"/hello\")",
				"backend-id": "hello"}}})

	registry := &mock.MiddlewareRegistry{}
	dispatcher := dispatch.Make()
	settingsSource := settings.MakeSource(mockDataClient, registry, dispatcher)

	proxy := proxy.Make(settingsSource)

	settingsChan := make(chan skipper.Settings)
	dispatcher.Subscribe(settingsChan)
	settings := waitForInitialSettings(settingsChan)

	// todo: print only in dev environment
	address := settings.Address()
	fmt.Printf("listening on %v\n", address)

	log.Fatal(http.ListenAndServe(address, proxy))
}
