package dispatch

import "testing"
import "skipper/skipper"
import "time"
import "net/http"

type testSettings struct{}

func (ts *testSettings) Route(*http.Request) (skipper.Route, error) { return nil, nil }
func (ts *testSettings) Address() string                            { return "" }

func checkDoneHelper(t *testing.T, remaining int, s, ss skipper.Settings) int {
	if ss != s {
		t.Error("wrong settings received")
		return 0
	}

	return remaining - 1
}

func TestForwardsAsPushed(t *testing.T) {
	sd := Make()

	sb := make(chan skipper.Settings)
	sd.Subscribe(sb)

	s := &testSettings{}
	sd.Push() <- s

	select {
	case ss := <-sb:
		if ss != s {
			t.Error("wrong settings received")
		}
	case <-time.After(15 * time.Millisecond):
		t.Error("didn't receive settings")
	}
}

func TestForwardOnSubscription(t *testing.T) {
	sd := Make()

	s := &testSettings{}
	sd.Push() <- s

	sb := make(chan skipper.Settings)
	sd.Subscribe(sb)

	select {
	case ss := <-sb:
		if ss != s {
			t.Error("wrong settings received")
		}
	case <-time.After(15 * time.Millisecond):
		t.Error("didn't receive settings")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	sd := Make()

	sbbefore := make(chan skipper.Settings)
	sd.Subscribe(sbbefore)

	s := &testSettings{}
	sd.Push() <- s

	sbafter := make(chan skipper.Settings)
	sd.Subscribe(sbafter)

	remaining := 2
	for {
		select {
		case <-time.After(15 * time.Millisecond):
			t.Error("didn't receive settings")
			return
		case ss := <-sbafter:
			remaining = checkDoneHelper(t, remaining, s, ss)
			if remaining <= 0 {
				return
			}
		case ss := <-sbbefore:
			remaining = checkDoneHelper(t, remaining, s, ss)
			if remaining <= 0 {
				return
			}
		}
	}
}

func TestPushNewSettings(t *testing.T) {
	sd := Make()

	sb := make(chan skipper.Settings)
	sd.Subscribe(sb)

	s0 := &testSettings{}
	sd.Push() <- s0

	select {
	case ss := <-sb:
		if ss != s0 {
			t.Error("wrong settings received")
		}
	case <-time.After(15 * time.Millisecond):
		t.Error("didn't receive settings")
	}

	s1 := &testSettings{}
	sd.Push() <- s1

	select {
	case ss := <-sb:
		if ss != s1 {
			t.Error("wrong settings received")
		}
	case <-time.After(15 * time.Millisecond):
		t.Error("didn't receive settings")
	}
}

func TestReceiveMultipleTimes(t *testing.T) {
	sd := Make()

	sb := make(chan skipper.Settings)
	sd.Subscribe(sb)

	s := &testSettings{}
	sd.Push() <- s

	remaining := 2
	for {
		select {
		case ss := <-sb:
			remaining = checkDoneHelper(t, remaining, s, ss)
			if remaining <= 0 {
				return
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

	s0 := &testSettings{}
	sd.Push() <- s0

	sb := make(chan skipper.Settings, bufSize)
	sd.Subscribe(sb)

	s1 := &testSettings{}
	sd.Push() <- s1

	remaining := 2 * bufSize
	for {
		select {
		case ss := <-sb:
			s := s0
			if remaining <= bufSize {
				s = s1
			}

			remaining = checkDoneHelper(t, remaining, s, ss)
			if remaining <= 0 {
				return
			}
		case <-time.After(15 * time.Millisecond):
			t.Error("didn't receive settings")
			return
		}
	}
}
