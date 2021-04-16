// +build !race redis

package proxy_test

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	ratelimitfilters "github.com/zalando/skipper/filters/ratelimit"
	snet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/redistest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/ratelimit"
)

type (
	countingBackend struct {
		server   *httptest.Server
		requests int
	}
	countingBackends []*countingBackend
)

func (b *countingBackend) ServeHTTP(http.ResponseWriter, *http.Request) {
	b.requests++
}

func (b *countingBackend) String() string {
	return b.server.URL
}

func newCountingBackend() *countingBackend {
	b := &countingBackend{}
	b.server = httptest.NewServer(b)
	return b
}

func newCountingBackends(n int) (result countingBackends) {
	for i := 0; i < n; i++ {
		result = append(result, newCountingBackend())
	}
	return
}

func (backends countingBackends) close() {
	for _, b := range backends {
		b.server.Close()
	}
}

func (backends countingBackends) endpoints() (result []string) {
	for _, b := range backends {
		result = append(result, b.server.URL)
	}
	return
}

func (backends countingBackends) String() string {
	var urls []string
	for _, e := range backends.endpoints() {
		urls = append(urls, `"`+e+`"`)
	}
	return strings.Join(urls, ",")
}

// Round robin distributes requests evenly between backends,
// test each backend gets exactly max hits before limit kicks-in
func TestBackendRateLimitRoundRobin(t *testing.T) {
	const (
		nBackends  = 3
		maxHits    = 7
		timeWindow = 10 * time.Second
	)

	filterRegistry := builtin.MakeRegistry()
	filterRegistry.Register(ratelimitfilters.NewBackendRatelimit())

	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	ratelimitRegistry := ratelimit.NewSwarmRegistry(nil, &snet.RedisOptions{Addrs: []string{redisAddr}})
	defer ratelimitRegistry.Close()

	backends := newCountingBackends(nBackends)
	defer backends.close()

	doc := fmt.Sprintf(`* -> backendRatelimit("testapi", %d, "%s") -> <roundRobin, %v>`, maxHits, timeWindow.String(), backends)
	r, err := eskip.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}

	p := proxytest.WithParams(filterRegistry, proxy.Params{RateLimiters: ratelimitRegistry}, r...)
	if testing.Verbose() {
		p.Log.Unmute()
	}
	defer p.Close()

	const totalMaxHits = nBackends * maxHits

	requestAndExpect(t, p.URL, totalMaxHits, http.StatusOK, nil)
	requestAndExpect(t, p.URL, 1, http.StatusServiceUnavailable, http.Header{"Content-Length": []string{"0"}})

	for _, b := range backends {
		if b.requests != maxHits {
			t.Errorf("Expected %d hits for backend %s, got: %d", maxHits, b, b.requests)
		}
	}
}

func TestBackendRateLimitScenarios(t *testing.T) {
	for _, ti := range []struct {
		name     string
		routes   string
		backends int
		requests map[string]int
		maxHits  int
	}{
		{
			"single route with one backend",
			`* -> backendRatelimit("testapi", 7, "10s") -> $backends`,
			1,
			map[string]int{"/": 10},
			7,
		},
		{
			"single route with three backends, random",
			`* -> backendRatelimit("testapi", 7, "10s") -> <random, $backends>`,
			3,
			map[string]int{"/": 30},
			7,
		},
		{
			"single route with three backends, roundRobin",
			`* -> backendRatelimit("testapi", 7, "10s") -> <roundRobin, $backends>`,
			3,
			map[string]int{"/": 30},
			7,
		},
		{
			"single route with three backends, consistentHash",
			`* -> backendRatelimit("testapi", 7, "10s") -> <consistentHash, $backends>`,
			3,
			map[string]int{"/": 30},
			7,
		},
		{
			"two routes with three backends and common limit",
			`api1: Path("/api1") -> backendRatelimit("api", 7, "10s") -> <random, $backends>;
			api2: Path("/api2") -> backendRatelimit("api", 7, "10s") -> <random, $backends>`,
			3,
			map[string]int{"/api1": 15, "/api2": 15},
			7,
		},
		{
			"two routes with three backends and separate limits",
			`api1: Path("/api1") -> backendRatelimit("api1", 4, "10s") -> <random, $backends>;
			api2: Path("/api2") -> backendRatelimit("api2", 8, "10s") -> <random, $backends>`,
			3,
			map[string]int{"/api1": 20, "/api2": 30},
			4 + 8,
		},
	} {
		t.Run(ti.name, func(t *testing.T) {
			filterRegistry := builtin.MakeRegistry()
			filterRegistry.Register(ratelimitfilters.NewBackendRatelimit())

			redisAddr, done := redistest.NewTestRedis(t)
			defer done()

			ratelimitRegistry := ratelimit.NewSwarmRegistry(nil, &snet.RedisOptions{Addrs: []string{redisAddr}})
			defer ratelimitRegistry.Close()

			backends := newCountingBackends(ti.backends)
			defer backends.close()

			r, err := eskip.Parse(strings.ReplaceAll(ti.routes, "$backends", backends.String()))
			if err != nil {
				t.Fatal(err)
			}

			p := proxytest.WithParams(filterRegistry, proxy.Params{RateLimiters: ratelimitRegistry}, r...)
			if testing.Verbose() {
				p.Log.Unmute()
			}
			defer p.Close()

			var urls []string
			for path, count := range ti.requests {
				for i := 0; i < count; i++ {
					urls = append(urls, p.URL+path)
				}
			}
			rand.Shuffle(len(urls), func(i, j int) { urls[i], urls[j] = urls[j], urls[i] })

			for _, url := range urls {
				rsp, err := http.Get(url)
				if err != nil {
					t.Fatalf("%s: %v", url, err)
				}
				defer rsp.Body.Close()

				if rsp.StatusCode != http.StatusOK && rsp.StatusCode != http.StatusServiceUnavailable {
					t.Fatalf("%s: unexpected status %d", url, rsp.StatusCode)
				}
			}

			for _, b := range backends {
				if b.requests > ti.maxHits {
					t.Errorf("Backend %s received %d above max hits %d ", b, b.requests, ti.maxHits)
				}
			}
		})
	}
}
