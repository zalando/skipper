package dispatch

import "skipper/skipper"

type fan struct {
	in  chan skipper.Settings
	out chan<- skipper.Settings
}

type dispatcher struct {
	push          chan skipper.Settings
	addSubscriber chan *fan
}

func makeFan(out chan<- skipper.Settings) *fan {
	f := &fan{make(chan skipper.Settings), out}
	go func() {
		s := <-f.in
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
		addSubscriber: make(chan *fan)}
	go func() {
		var settings skipper.Settings
		var fans []*fan

		for {
			select {
			case settings = <-d.push:
				for _, f := range fans {
					f.in <- settings
				}
			case f := <-d.addSubscriber:
				fans = append(fans, f)
				f.in <- settings
			}
		}
	}()

	return d
}

func (d *dispatcher) Subscribe(c chan<- skipper.Settings) {
	d.addSubscriber <- makeFan(c)
}

func (d *dispatcher) Push() chan<- skipper.Settings {
	return d.push
}
