package proxy_test

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

// simple crash test only, use utilities in skptesting
// for benchmarking.
func TestIdleConns(t *testing.T) {
	doc := func(l int) []byte {
		b := make([]byte, l)
		n, err := rand.Read(b)
		if err != nil || n != l {
			t.Fatal("failed to generate doc", err, n, l)
		}

		return b
	}

	server := func(doc []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write(doc)
		}))
	}

	d0 := doc(128)
	s0 := server(d0)
	defer s0.Close()

	d1 := doc(256)
	s1 := server(d1)
	defer s1.Close()

	const (
		closePeriod        = 100 * time.Millisecond
		concurrentRequests = 10
	)

	for _, ti := range []struct {
		msg            string
		idleConns      int
		closeIdleConns time.Duration
	}{{
		"negative idle (default), negative close (none)",
		-1,
		-1 * closePeriod,
	}, {
		"zero idle (default), negative close (none)",
		0,
		-1 * closePeriod,
	}, {
		"small idle, negative close (none)",
		3,
		-1 * closePeriod,
	}, {
		"large idle, negative close (none)",
		256,
		-1 * closePeriod,
	}, {
		"negative idle (default), zero close (default)",
		-1,
		0,
	}, {
		"zero idle (default), zero close (default)",
		0,
		0,
	}, {
		"small idle, zero close (default)",
		3,
		0,
	}, {
		"large idle, zero close (default)",
		256,
		0,
	}, {
		"negative idle (default), close",
		-1,
		closePeriod,
	}, {
		"zero idle (default), close",
		0,
		closePeriod,
	}, {
		"small idle, close",
		3,
		closePeriod,
	}, {
		"large idle, close",
		256,
		closePeriod,
	}} {
		p := proxytest.NewOptions(nil,
			proxy.ProxyOptions{
				IdleConnectionsPerHost: ti.idleConns,
				CloseIdleConnsPeriod:   ti.closeIdleConns},
			&eskip.Route{Id: "s0", Path: "/s0", Backend: s0.URL},
			&eskip.Route{Id: "s1", Path: "/s1", Backend: s1.URL})
		defer p.Close()

		request := func(path string, doc []byte) {
			rsp, err := http.Get(p.URL + path)
			if err != nil {
				t.Fatal(ti.msg, "failed to make request", err)
				return
			}

			defer rsp.Body.Close()
			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				t.Fatal(ti.msg, "failed to read response", err)
			}

			if !bytes.Equal(b, doc) {
				t.Fatal(ti.msg, "failed to read response, invalid content", len(b), len(doc))
			}
		}

		stop := make(chan struct{})
		wg := sync.WaitGroup{}

		runRequests := func(path string, doc []byte) {
			wg.Add(1)
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					request(path, doc)
				}
			}
		}

		for i := 0; i < concurrentRequests; i++ {
			go runRequests("/s0", d0)
			go runRequests("/s1", d1)
		}

		<-time.After(10 * closePeriod)
		close(stop)
		wg.Wait()
	}
}
