package serve

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testDelay = 12 * time.Millisecond

func TestPipe(t *testing.T) {
	testError := errors.New("test error")
	d := []string{"foo", "bar", "baz"}
	b := NewPipedBody()

	go func() {
		for _, di := range d {
			b.Write([]byte(di))
		}

		b.CloseWithError(testError)
	}()

	p := make([]byte, 6)
	var db []string
	for {
		n, err := b.Read(p)
		if err != nil {
			if err != testError {
				t.Error("invalid error")
			}

			break
		}

		db = append(db, string(p[:n]))
	}

	if strings.Join(db, "-") != strings.Join(d, "-") {
		t.Error("piping failed")
	}
}

func TestBlock(t *testing.T) {
	for _, ti := range []struct {
		msg             string
		sleep           time.Duration
		status          int
		body            string
		ret             bool
		blockForever    bool
		expectedTimeout time.Duration
		timeout         time.Duration
	}{{
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
		done := make(chan struct{})
		quit := make(chan struct{})
		go func() {
			ServeResponse(nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		}()

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
				t.Error(ti.msg, "failed to block")
			} else {
				close(quit)
			}
		case <-eto:
			close(quit)
		case <-to:
			t.Error(ti.msg, "timeout")
			close(quit)
		}
	}
}

func TestServe(t *testing.T) {
	parts := []string{"foo", "bar", "baz"}
	req := &http.Request{}
	rsp := ServeResponse(req, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r != req {
			t.Error("invalid request object")
		}

		w.Header().Set("X-Test-Header", "test-value")
		w.WriteHeader(http.StatusTeapot)
		for _, p := range parts {
			w.Write([]byte(p))
		}
	}))

	if rsp.StatusCode != http.StatusTeapot {
		t.Error("failed to serve status")
	}

	if rsp.Header.Get("X-Test-Header") != "test-value" {
		t.Error("failed to serve header")
	}

	b, err := ioutil.ReadAll(rsp.Body)
	if err != nil || string(b) != strings.Join(parts, "") {
		t.Error("failed to serve body")
	}
}

func TestStreamBody(t *testing.T) {
	parts := []string{"foo", "bar", "baz"}
	signal := make(chan int)
	rsp := ServeResponse(nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		n, err := rsp.Body.Read(b)
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
