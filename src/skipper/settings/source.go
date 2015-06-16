package settings

import "skipper/skipper"

type source struct {
	dispatcher skipper.SettingsDispatcher
}

func MakeSource(
	dc skipper.DataClient,
	mwr skipper.MiddlewareRegistry,
	sd skipper.SettingsDispatcher) skipper.SettingsSource {

	s := &source{sd}
	go func() {
		for {
			data := <-dc.Receive()
			settings := processRaw(data, mwr)
			s.dispatcher.Push() <- settings
		}
	}()

	return s
}

func (s *source) Subscribe(c chan<- skipper.Settings) {
	s.dispatcher.Subscribe(c)
}
