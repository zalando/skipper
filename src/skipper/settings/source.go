package settings

import (
	"log"
	"skipper/skipper"
)

type source struct {
	dispatcher skipper.SettingsDispatcher
}

// creates a skipper.SettingsSource instance.
// expects an instance of the etcd client, a middleware registry and a settings dispatcher.
func MakeSource(
	dc skipper.DataClient,
	mwr skipper.MiddlewareRegistry,
	sd skipper.SettingsDispatcher) skipper.SettingsSource {

	s := &source{sd}
	go func() {
		for {
			data := <-dc.Receive()

			settings, err := processRaw(data, mwr)
			if err != nil {
				log.Println(err)
				continue
			}

			s.dispatcher.Push() <- settings
		}
	}()

	return s
}

func (s *source) Subscribe(c chan<- skipper.Settings) {
	s.dispatcher.Subscribe(c)
}
