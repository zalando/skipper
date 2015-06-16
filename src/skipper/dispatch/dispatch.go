package dispatch

import "skipper/skipper"

type fan struct {
	in  chan skipper.Settings
	out chan<- skipper.Settings
}

type dispatcher struct {
	push          chan skipper.Settings
	addSubscriber chan chan<- skipper.Settings
}

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

func (d *dispatcher) Subscribe(c chan<- skipper.Settings) {
	d.addSubscriber <- c
}

func (d *dispatcher) Push() chan<- skipper.Settings {
	return d.push
}
