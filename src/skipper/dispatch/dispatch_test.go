package dispatch

import (
	"skipper/mock"
	"skipper/skipper"
	"testing"
	"time"
)

func TestForwardsAsPushed(t *testing.T) {
	sd := Make()

	sb := make(chan skipper.Settings)
	sd.Subscribe(sb)

	s := &mock.Settings{}
	sd.Push() <- s

	for {
		select {
		case ss := <-sb:
			if ss != nil {
				return
			}
		case <-time.After(15 * time.Millisecond):
			t.Error("didn't receive settings")
			return
		}
	}
}

func TestForwardOnSubscription(t *testing.T) {
	sd := Make()

	s := &mock.Settings{}
	sd.Push() <- s

	sb := make(chan skipper.Settings)
	sd.Subscribe(sb)

	for {
		select {
		case ss := <-sb:
			if ss != nil {
				return
			}
		case <-time.After(15 * time.Millisecond):
			t.Error("didn't receive settings")
		}
	}
}

func TestMultipleSubscribers(t *testing.T) {
	sd := Make()

	sbbefore := make(chan skipper.Settings)
	sd.Subscribe(sbbefore)
	rbefore := false

	s := &mock.Settings{}
	sd.Push() <- s

	sbafter := make(chan skipper.Settings)
	sd.Subscribe(sbafter)
	rafter := false

	for {
		select {
		case <-time.After(15 * time.Millisecond):
			t.Error("didn't receive settings")
			return
		case <-sbafter:
			rafter = true
			if rbefore {
				return
			}
		case <-sbbefore:
			rbefore = true
			if rafter {
				return
			}
		}
	}
}

func TestPushNewSettings(t *testing.T) {
	sd := Make()

	sb := make(chan skipper.Settings)
	sd.Subscribe(sb)

	s0 := &mock.Settings{}
	sd.Push() <- s0

	func() {
		for {
			select {
			case ss := <-sb:
				if ss == s0 {
					return
				}
			case <-time.After(15 * time.Millisecond):
				t.Error("didn't receive settings")
			}
		}
	}()

	s1 := &mock.Settings{}
	sd.Push() <- s1

	func() {
		for {
			select {
			case ss := <-sb:
				if ss == s1 {
					return
				}
			case <-time.After(15 * time.Millisecond):
				t.Error("didn't receive settings")
			}
		}
	}()
}

func TestReceiveMultipleTimes(t *testing.T) {
	sd := Make()

	sb := make(chan skipper.Settings)
	sd.Subscribe(sb)

	s := &mock.Settings{}
	sd.Push() <- s

	r := false
	for {
		select {
		case ss := <-sb:
			if ss == s {
				if r {
					return
				}

				r = true
			}
		case <-time.After(15 * time.Millisecond):
			t.Error("didn't receive settings")
			return
		}
	}
}

func TestReceiveOnBufferedChannel(t *testing.T) {
	const bufSize = 2
	sd := Make()

	s0 := &mock.Settings{}
	sd.Push() <- s0

	sb := make(chan skipper.Settings, bufSize)
	sd.Subscribe(sb)

	s1 := &mock.Settings{}
	sd.Push() <- s1

	for {
		time.Sleep(1 * time.Millisecond)
		select {
		case ss := <-sb:
			if ss == s1 {
				return
			}
		case <-time.After(15 * time.Millisecond):
			t.Error("didn't receive settings")
			return
		}
	}
}
