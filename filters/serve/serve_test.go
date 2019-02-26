package serve

import (
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/filtertest"
)

const testDelay = 12 * time.Millisecond

type testItem struct {
	msg             string
	sleep           time.Duration
	status          int
	body            string
	ret             bool
	blockForever    bool
	expectedTimeout time.Duration
	timeout         time.Duration
}

func TestBlock(t *testing.T) {
	for _, ti := range []testItem{{
		msg:             "block forever",
		expectedTimeout: testDelay,
	}, {
		msg:     "block until header",
		sleep:   testDelay,
		status:  http.StatusTeapot,
		timeout: 9 * testDelay,
	}, {
		msg:     "block until body",
		sleep:   testDelay,
		body:    "foo",
		timeout: 9 * testDelay,
	}, {
		msg:     "block until return",
		sleep:   testDelay,
		ret:     true,
		timeout: 9 * testDelay,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			done := make(chan struct{})
			quit := make(chan struct{})
			ctx := &filtertest.Context{}
			go func(ti testItem) {
				ServeHTTP(ctx, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if ti.sleep > 0 {
						time.Sleep(ti.sleep)
					}

					if ti.status > 0 {
						w.WriteHeader(ti.status)
					}

					if ti.body != "" {
						w.Write([]byte(ti.body))
					}

					if !ti.ret {
						<-quit
					}
				}))

				close(done)
			}(ti)

			var eto, to <-chan time.Time
			if ti.expectedTimeout > 0 {
				eto = time.After(ti.expectedTimeout)
			}
			if ti.timeout > 0 {
				to = time.After(ti.timeout)
			}

			select {
			case <-done:
				if ti.blockForever {
					t.Error("failed to block")
				} else {
					close(quit)
				}
			case <-eto:
				close(quit)
			case <-to:
				t.Error("timeout")
				close(quit)
			}
		})
	}
}

func TestServe(t *testing.T) {
	parts := []string{"foo", "bar", "baz"}
	ctx := &filtertest.Context{FRequest: &http.Request{}}
	ServeHTTP(ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r != ctx.Request() {
			t.Error("invalid request object")
		}

		w.Header().Set("X-Test-Header", "test-value")
		w.WriteHeader(http.StatusTeapot)
		for _, p := range parts {
			w.Write([]byte(p))
		}
	}))

	if ctx.Response().StatusCode != http.StatusTeapot {
		t.Error("failed to serve status")
	}

	if ctx.Response().Header.Get("X-Test-Header") != "test-value" {
		t.Error("failed to serve header")
	}

	b, err := ioutil.ReadAll(ctx.Response().Body)
	if err != nil || string(b) != strings.Join(parts, "") {
		t.Error("failed to serve body")
	}
}

func TestStreamBody(t *testing.T) {
	parts := []string{"foo", "bar", "baz"}
	signal := make(chan int)
	ctx := &filtertest.Context{}
	ServeHTTP(ctx, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		for i := range signal {
			if i < len(parts) {
				w.Write([]byte(parts[i]))
			}
		}
	}))

	var rparts []string
	for i, p := range parts {
		signal <- i
		b := make([]byte, len(p))
		n, err := ctx.Response().Body.Read(b)
		if n != len(p) || err != nil && err != io.EOF {
			t.Error("failed to stream body, read error", n, err)
			close(signal)
			return
		}

		rparts = append(rparts, string(b))
	}

	if strings.Join(rparts, "") != strings.Join(parts, "") {
		t.Error("failed to stream body, invalid output")
	}
}
