package circuit

import (
	"math/rand"
	"testing"
	"time"
)

func times(n int, f func()) {
	for n > 0 {
		f()
		n--
	}
}

func createDone(t *testing.T, success bool, b *Breaker) func() {
	return func() {
		if t.Failed() {
			return
		}

		done, ok := b.Allow()
		if !ok {
			t.Error("breaker is unexpectedly open")
			return
		}

		done(success)
	}
}

func succeed(t *testing.T, b *Breaker) func() { return createDone(t, true, b) }
func fail(t *testing.T, b *Breaker) func()    { return createDone(t, false, b) }
func failOnce(t *testing.T, b *Breaker)       { fail(t, b)() }

func checkClosed(t *testing.T, b *Breaker) {
	if _, ok := b.Allow(); !ok {
		t.Error("breaker is not closed")
	}
}

func checkOpen(t *testing.T, b *Breaker) {
	if _, ok := b.Allow(); ok {
		t.Error("breaker is not open")
	}
}

func TestConsecutiveFailures(t *testing.T) {
	s := BreakerSettings{
		Type:             ConsecutiveFailures,
		Failures:         3,
		HalfOpenRequests: 3,
		Timeout:          15 * time.Millisecond,
	}

	waitTimeout := func() {
		time.Sleep(s.Timeout)
	}

	t.Run("new breaker closed", func(t *testing.T) {
		b := newBreaker(s)
		checkClosed(t, b)
	})

	t.Run("does not open on not enough failures", func(t *testing.T) {
		b := newBreaker(s)
		times(s.Failures-1, fail(t, b))
		checkClosed(t, b)
	})

	t.Run("open on failures", func(t *testing.T) {
		b := newBreaker(s)
		times(s.Failures, fail(t, b))
		checkOpen(t, b)
	})

	t.Run("go half open, close after required successes", func(t *testing.T) {
		b := newBreaker(s)
		times(s.Failures, fail(t, b))
		waitTimeout()
		times(s.HalfOpenRequests, succeed(t, b))
		checkClosed(t, b)
	})

	t.Run("go half open, reopen after a fail within the required successes", func(t *testing.T) {
		b := newBreaker(s)
		times(s.Failures, fail(t, b))
		waitTimeout()
		times(s.HalfOpenRequests-1, succeed(t, b))
		failOnce(t, b)
		checkOpen(t, b)
	})
}

func TestRateBreaker(t *testing.T) {
	s := BreakerSettings{
		Type:             FailureRate,
		Window:           6,
		Failures:         3,
		HalfOpenRequests: 3,
		Timeout:          3 * time.Millisecond,
	}

	t.Run("new breaker closed", func(t *testing.T) {
		b := newBreaker(s)
		checkClosed(t, b)
	})

	t.Run("doesn't open if failure count is not within a window", func(t *testing.T) {
		b := newBreaker(s)
		times(1, fail(t, b))
		times(2, succeed(t, b))
		checkClosed(t, b)
		times(1, fail(t, b))
		times(2, succeed(t, b))
		checkClosed(t, b)
		times(1, fail(t, b))
		times(2, succeed(t, b))
		checkClosed(t, b)
	})

	t.Run("opens on reaching the rate", func(t *testing.T) {
		b := newBreaker(s)
		times(s.Window, succeed(t, b))
		times(s.Failures, fail(t, b))
		checkOpen(t, b)
	})
}

// no checks, used for race detector
func TestRateBreakerFuzzy(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	const (
		concurrentRequests = 64
		requestDuration    = 6 * time.Microsecond
		requestDelay       = 6 * time.Microsecond
		duration           = 3 * time.Second
	)

	s := BreakerSettings{
		Type:             FailureRate,
		Window:           300,
		Failures:         120,
		HalfOpenRequests: 12,
		Timeout:          3 * time.Millisecond,
	}

	b := newBreaker(s)

	stop := make(chan struct{})

	successChance := func() bool {
		return rand.Intn(s.Window) > s.Failures
	}

	runAgent := func() {
		for {
			select {
			case <-stop:
			default:
			}

			done, ok := b.Allow()
			time.Sleep(requestDuration)
			if ok {
				done(successChance())
			}

			time.Sleep(requestDelay)
		}
	}

	time.AfterFunc(duration, func() {
		close(stop)
	})

	for range concurrentRequests {
		go runAgent()
	}

	<-stop
}

func TestSettingsString(t *testing.T) {
	s := BreakerSettings{
		Type:             FailureRate,
		Host:             "www.example.org",
		Failures:         30,
		Window:           300,
		Timeout:          time.Minute,
		HalfOpenRequests: 15,
		IdleTTL:          time.Hour,
	}

	ss := s.String()
	expect := "type=rate,host=www.example.org,window=300,failures=30,timeout=1m0s,half-open-requests=15,idle-ttl=1h0m0s"
	if ss != expect {
		t.Error("invalid breaker settings string")
		t.Logf("got     : %s", ss)
		t.Logf("expected: %s", expect)
	}
}
