// The dispatch package provides a settings dispatcher (skipper.SettingsDispatcher) implementation.
package dispatch

import "github.com/zalando/skipper/skipper"

type fan struct {
	in  chan skipper.Settings
	out chan<- skipper.Settings
}

type dispatcher struct {
	push          chan skipper.Settings
	addSubscriber chan chan<- skipper.Settings
}

// constantly feeds the 'out' channel with the current settings
func makeFan(s skipper.Settings, out chan<- skipper.Settings) *fan {
	f := &fan{make(chan skipper.Settings), out}
	go func() {
		for {
			select {
			case s = <-f.in:
			case f.out <- s:
			}
		}
	}()

	return f
}

// Creates a dispatcher object for settings. To send the latest available settings to any request processing or other
// goroutines without blocking, clients who use the Subscribe method will always receive the latest settings,
// while goroutines responsible to process the incoming config data and create the next valid settings object
// can dispatch the new settings with the Push method.
func Make() skipper.SettingsDispatcher {
	d := &dispatcher{
		push:          make(chan skipper.Settings),
		addSubscriber: make(chan chan<- skipper.Settings)}
	go func() {
		var settings skipper.Settings
		var fans []*fan

		for {
			select {
			case s := <-d.push:
				settings = s
				for _, f := range fans {
					f.in <- settings
				}
			case c := <-d.addSubscriber:
				fans = append(fans, makeFan(settings, c))
			}
		}
	}()

	return d
}

// Accepts a channel on which the calling code wants receive the the current Settings.
// It can be a good idea to use buffered channels in production environment.
func (d *dispatcher) Subscribe(c chan<- skipper.Settings) {
	d.addSubscriber <- c
}

// When new settings are ready, use the returned channel to propagate them.
func (d *dispatcher) Push() chan<- skipper.Settings {
	return d.push
}
