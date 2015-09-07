package settings

import (
	"github.com/zalando/skipper/requestmatch"
	"github.com/zalando/skipper/skipper"
	"github.com/zalando/skipper/dispatch"
	"log"
)

type source struct {
	dispatcher *dispatch.Dispatcher
}

// creates a skipper.SettingsSource instance.
// expects an instance of the etcd client, a filter registry and a dispatcher for settings.
func MakeSource(
	dc skipper.DataClient,
	fr skipper.FilterRegistry,
	ignoreTrailingSlash bool) skipper.SettingsSource {

	s := &source{&dispatch.Dispatcher{}}

	// create initial empty settings:
	rm, _ := requestmatch.Make(nil, false)
    s.dispatcher.Start()
	s.dispatcher.Push <- &settings{rm}

	go func() {
		for {
			data := <-dc.Receive()

			ss, err := processRaw(data, fr, ignoreTrailingSlash)
			if err != nil {
				log.Println(err)
				continue
			}

			s.dispatcher.Push <- ss
		}
	}()

	return s
}

func (s *source) Subscribe(c chan<- skipper.Settings) {
    ic := make(chan interface{})
	s.dispatcher.AddSubscriber <- ic
    go func() { c <- (<-ic).(skipper.Settings) }()
}
