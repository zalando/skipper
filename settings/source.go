package settings

import (
	"github.com/mailgun/route"
	"github.com/zalando/skipper/skipper"
	"log"
)

type source struct {
	dispatcher skipper.SettingsDispatcher
}

// creates a skipper.SettingsSource instance.
// expects an instance of the etcd client, a filter registry and a settings dispatcher.
func MakeSource(
	dc skipper.DataClient,
	mwr skipper.FilterRegistry,
	sd skipper.SettingsDispatcher) skipper.SettingsSource {

	// create initial empty settings:
	sd.Push() <- &settings{&mailgunRouter{route.New()}}

	s := &source{sd}
	go func() {
		for {
			data := <-dc.Receive()

			ss, err := processRaw(data, mwr)
			if err != nil {
				log.Println(err)
				continue
			}

			s.dispatcher.Push() <- ss
		}
	}()

	return s
}

func (s *source) Subscribe(c chan<- skipper.Settings) {
	s.dispatcher.Subscribe(c)
}
