package main

import "log"
import "net/http"
import "skipper/proxy"
import "skipper/settings"
import "skipper/skipper"
import "skipper/dispatch"
import "time"
import "fmt"

const startupSettingsTimeout = 1200 * time.Millisecond

func waitForInitialSettings(c <-chan skipper.Settings) skipper.Settings {
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
	mockDataClient := makeMockDataClient()
	registry := &mockMiddlewareRegistry{}

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
