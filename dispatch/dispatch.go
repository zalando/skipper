// Package dispatch provides a dispatcher between goroutines. It sends the latest available data to any
// goroutine without blocking through the channels passed through the AddSubscriber channel. This means
// that whenever a gorouting reads from this channel, it will received the current data. The next
// version of the dispatched data can be dispatched to the clients through the Push channel.
package dispatch

type fan struct {
	in  chan interface{}
	out chan<- interface{}
}

// Implements a dispatcher object. Use the Start method to start dispatching.
type Dispatcher struct {
	Push          chan interface{}
	AddSubscriber chan chan<- interface{}
}

// constantly feeds the 'out' channel with the current settings
func makeFan(data interface{}, out chan<- interface{}) *fan {
	f := &fan{make(chan interface{}), out}
	go func() {
		for {
            println("waiting for fan")
			select {
			case data = <-f.in:
                println("fan in done")
			case f.out <- data:
                println("fan out done")
			}
		}

        println("exiting here for some reason")
	}()

	return f
}

// Initializes the dispatcher and starts dispatching.
func (d *Dispatcher) Start() {
    if d.Push == nil {
        d.Push = make(chan interface{})
    }

    if d.AddSubscriber == nil {
        d.AddSubscriber = make(chan chan<- interface{})
    }

	go func() {
        var data interface{}
		var fans []*fan

		for {
			select {
			case data = <-d.Push:
                println("push", data)
				for _, f := range fans {
                    println("fanning in")
					f.in <- data
                    println("fanning in done")
				}
			case c := <-d.AddSubscriber:
                println("subscriber received")
				fans = append(fans, makeFan(data, c))
			}
		}
	}()
}
