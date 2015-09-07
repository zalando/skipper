package dispatch

import (
	"testing"
	"time"
)

func TestForwardAsPushed(t *testing.T) {
	d := &Dispatcher{}
	d.Start()

	sb := make(chan interface{})
	d.AddSubscriber <- sb

	data := 42
	d.Push <- data

	for {
		select {
		case dataBack := <-sb:
			if dataBackInt, ok := dataBack.(int); ok && dataBackInt == data {
				return
			}
		case <-time.After(15 * time.Millisecond):
			t.Error("timeout")
			return
		}
	}
}

func TestForwardOnSubscription(t *testing.T) {
	d := &Dispatcher{}
	d.Start()

	data := 42
	d.Push <- data

	sb := make(chan interface{})
	d.AddSubscriber <- sb

	for {
		select {
		case dataBack := <-sb:
			if dataBackInt, ok := dataBack.(int); ok && dataBackInt == data {
				return
			}
		case <-time.After(15 * time.Millisecond):
			t.Error("timeout")
			return
		}
	}
}

func TestMultipleSubscribers(t *testing.T) {
	d := &Dispatcher{}
	d.Start()

	sbbefore := make(chan interface{})
	d.AddSubscriber <- sbbefore
	receivedBefore := false

	data := 42
	d.Push <- data

	sbafter := make(chan interface{})
	d.AddSubscriber <- sbafter
	receivedAfter := false

	for {
		select {
		case <-time.After(15 * time.Millisecond):
			t.Error("timeout")
			return
		case dataBack := <-sbafter:
			if dataBackInt, ok := dataBack.(int); ok && dataBackInt == data {
				receivedAfter = true
				if receivedBefore {
					return
				}
			}
		case dataBack := <-sbbefore:
			if dataBackInt, ok := dataBack.(int); ok && dataBackInt == data {
				receivedBefore = true
				if receivedAfter {
					return
				}
			}
		}
	}
}

func TestPushNewData(t *testing.T) {
	d := &Dispatcher{}
	d.Start()

	sb := make(chan interface{})
	d.AddSubscriber <- sb

	data0 := 36
	d.Push <- data0

	go func() {
		for {
			select {
			case dataBack := <-sb:
				if dataBackInt, ok := dataBack.(int); ok && dataBackInt == data0 {
					return
				}
			case <-time.After(15 * time.Millisecond):
				t.Error("timeout")
				return
			}
		}
	}()

	data1 := 42
	d.Push <- data1

	go func() {
		for {
			select {
			case dataBack := <-sb:
				if dataBackInt, ok := dataBack.(int); ok && dataBackInt == data0 {
					return
				}
			case <-time.After(15 * time.Millisecond):
				t.Error("timeout")
				return
			}
		}
	}()
}

func TestReceiveMultipleTimes(t *testing.T) {
	d := &Dispatcher{}
	d.Start()

	sb := make(chan interface{})
	d.AddSubscriber <- sb

	data := 42
	d.Push <- data

	received := false
	for {
		select {
		case dataBack := <-sb:
			if dataBackInt, ok := dataBack.(int); ok && dataBackInt == data {
				if received {
					return
				}

				received = true
			}
		case <-time.After(15 * time.Millisecond):
			t.Error("timeout")
			return
		}
	}
}

func TestReceiveOnBufferedChannel(t *testing.T) {
	const bufSize = 2
	d := &Dispatcher{}
	d.Start()

	data0 := 42
	d.Push <- data0

	sb := make(chan interface{}, bufSize)
	d.AddSubscriber <- sb

	data1 := 42
	d.Push <- data1

	for {
		time.Sleep(3 * time.Millisecond)
		select {
		case dataBack := <-sb:
			if dataBackInt, ok := dataBack.(int); ok && dataBackInt == data1 {
				return
			}
		case <-time.After(15 * time.Millisecond):
			t.Error("timeout")
			return
		}
	}
}
