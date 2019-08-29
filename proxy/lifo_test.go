package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/scheduler"
)

type counter chan int

func newCounter() counter {
	c := make(counter, 1)
	c <- 0
	return c
}

func (c counter) inc() {
	c <- <-c + 1
}

func (c counter) reset() {
	<-c
	c <- 0
}

func (c counter) value() int {
	v := <-c
	c <- v
	return v
}

func (c counter) waitFor(v int) {
	for c.value() != v {
	}
}

func TestGlobalLIFO(t *testing.T) {
	finish := make(chan struct{})
	backendReached := newCounter()
	b := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		backendReached.inc()
		f := finish // this level of race should be fine for the tests
		<-f
	}))

	reg := scheduler.NewRegistry()
	lifo := reg.Global(scheduler.Config{MaxConcurrency: 3, MaxQueueSize: 3, Timeout: 10 * time.Millisecond})

	waitForStatus := func(s scheduler.QueueStatus) {
		for lifo.Status() != s {
		}
	}

	tp, err := newTestProxyWithParams(fmt.Sprintf(`* -> "%s"`, b.URL), Params{GlobalLIFO: lifo})
	if err != nil {
		t.Fatal(err)
	}

	defer tp.close()
	failures := newCounter()
	request := func() {
		r := httptest.NewRecorder()
		tp.proxy.ServeHTTP(r, &http.Request{URL: &url.URL{}})
		if r.Code != http.StatusOK {
			failures.inc()
		}
	}

	releaseBackend := func() {
		close(finish)
		backendReached.reset()
		failures.reset()
		waitForStatus(scheduler.QueueStatus{})
		finish = make(chan struct{})
	}

	t.Run("pass all requests", func(t *testing.T) {
		defer releaseBackend()
		for i := 0; i < 3; i++ {
			go request()
		}

		backendReached.waitFor(3)
		waitForStatus(scheduler.QueueStatus{ActiveRequests: 3})
	})

	t.Run("queue requests", func(t *testing.T) {
		defer releaseBackend()
		for i := 0; i < 6; i++ {
			go request()
		}

		backendReached.waitFor(3)
		waitForStatus(scheduler.QueueStatus{ActiveRequests: 3, QueuedRequests: 3})
	})

	t.Run("reject requests", func(t *testing.T) {
		defer releaseBackend()
		for i := 0; i < 9; i++ {
			go request()
		}

		backendReached.waitFor(3)
		failures.waitFor(3)
		waitForStatus(scheduler.QueueStatus{ActiveRequests: 3, QueuedRequests: 3})
	})

	t.Run("timeout requests", func(t *testing.T) {
		defer releaseBackend()
		for i := 0; i < 6; i++ {
			go request()
		}

		backendReached.waitFor(3)
		waitForStatus(scheduler.QueueStatus{ActiveRequests: 3, QueuedRequests: 3})
		time.Sleep(10 * time.Millisecond)
		waitForStatus(scheduler.QueueStatus{ActiveRequests: 3, QueuedRequests: 0})
		failures.waitFor(3)
	})
}
