package settings

import (
	"github.com/zalando/skipper/dispatch"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/requestmatch"
	"log"
)

type Source struct {
	dispatcher *dispatch.Dispatcher
}

type DataClient interface {
	Receive() <-chan string
}

// creates a skipper.SettingsSource instance.
// expects an instance of the etcd client, a filter registry and a dispatcher for settings.
func MakeSource(dc DataClient, fr filters.Registry, ignoreTrailingSlash bool) *Source {
    if fr == nil {
        fr = make(filters.Registry)
    }

	s := &Source{&dispatch.Dispatcher{}}

	// create initial empty settings:
	rm, _ := requestmatch.Make(nil, false)
	s.dispatcher.Start()
	s.dispatcher.Push <- &Settings{rm}

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

func (s *Source) Subscribe(c chan<- *Settings) {
	ic := make(chan interface{})
	s.dispatcher.AddSubscriber <- ic
	go func() {
		for {
			c <- (<-ic).(*Settings)
		}
	}()
}
