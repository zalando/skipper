package proxy_test

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

/*
Creating a separate test harness for the fade-in functionality, because the existing test proxy doesn't support
multiple proxy instances and should have been wrapped anyway, while the changes in the routing in these tests
are consequences to events happening to the backend instances, and therefore the test data client should have
been wrapped, too. The harness implements the following architecture:

	client -> proxy -> data client <- backend
	          |-> proxy instance 1    |-> backend instance 1
	          |-> proxy instance 2    |-> backend instance 1
	          \-> proxy instance 2    \-> backend instance 1
*/

const (
	minStats        = 2048
	statBucketCount = 8
	epsilon         = 0.2
	easeClient      = 100 * time.Microsecond
	statsTimeout    = 3 * time.Second
)

type fadeInDataClient struct {
	reset, update chan []*eskip.Route
}

type fadeInBackend struct {
	test      *testing.T
	clients   []fadeInDataClient
	instances []*httptest.Server
}

type fadeInProxyInstance struct {
	proxy  *proxy.Proxy
	server *httptest.Server
}

type fadeInProxy struct {
	test      *testing.T
	mx        *sync.Mutex
	backend   *fadeInBackend
	instances []*fadeInProxyInstance
}

type stat struct {
	timestamp time.Time
	status    int
	endpoint  string
	err       error
}

type statRequest struct {
	minStats int
	response chan []stat
}

type fadeInClient struct {
	test               *testing.T
	proxy              *fadeInProxy
	httpClient         *http.Client
	statRequests       chan statRequest
	resetStatsRequests chan struct{}
	quit               chan struct{}
}

func randomURLs(t *testing.T, n int) []string {
	var u []string
	for i := 0; i < n; i++ {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
			return nil
		}

		u = append(u, fmt.Sprintf("http://%s", l.Addr()))
		l.Close()
	}

	return u
}

func createDataClient(r ...*eskip.Route) fadeInDataClient {
	var c fadeInDataClient
	c.reset = make(chan []*eskip.Route, 1)
	c.update = make(chan []*eskip.Route, 1)
	c.reset <- r
	return c
}

func (c fadeInDataClient) LoadAll() ([]*eskip.Route, error) {
	r := <-c.reset
	c.reset <- r
	return r, nil
}

func (c fadeInDataClient) LoadUpdate() ([]*eskip.Route, []string, error) {
	return <-c.update, nil, nil
}

// startBackend starts a backend representing 0 or more endpoints, added in a separate step.
func startBackend(t *testing.T) *fadeInBackend {
	return &fadeInBackend{test: t}
}

func (b *fadeInBackend) route() *eskip.Route {
	r := &eskip.Route{
		Id:          "fadeInRoute",
		BackendType: eskip.LBBackend,
	}

	for _, i := range b.instances {
		r.LBEndpoints = append(r.LBEndpoints, i.URL)
	}

	return r
}

func (b *fadeInBackend) createDataClient() routing.DataClient {
	c := createDataClient(b.route())
	b.clients = append(b.clients, c)
	return c
}

func (b *fadeInBackend) addInstances(u ...string) {
	var instances []*httptest.Server
	for _, ui := range u {
		func(u string) {
			uu, err := url.Parse(u)
			if err != nil {
				b.test.Fatal(err)
			}

			instance := httptest.NewUnstartedServer(nil)
			instance.Config = &http.Server{
				Addr: uu.Host,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("X-Backend-Endpoint", u)
				}),
			}

			instance.Start()
			instances = append(instances, instance)
		}(ui)
	}

	b.instances = append(b.instances, instances...)
	r := []*eskip.Route{b.route()}
	for _, c := range b.clients {
		<-c.reset
		select {
		case <-c.update:
		default:
		}

		c.reset <- r
		c.update <- r
	}
}

func (b *fadeInBackend) close() {
	for _, i := range b.instances {
		i.Close()
	}
}

func (p *fadeInProxyInstance) close() {
	p.server.Close()
	p.proxy.Close()
}

// startProxy starts a proxy representing 0 or more proxy instances, added in a separate step.
func startProxy(t *testing.T, b *fadeInBackend) *fadeInProxy {
	return &fadeInProxy{
		test:    t,
		mx:      &sync.Mutex{},
		backend: b,
	}
}

func (p *fadeInProxy) addInstances(n int) {
	p.mx.Lock()
	defer p.mx.Unlock()

	for i := 0; i < n; i++ {
		client := p.backend.createDataClient()
		rt := routing.New(routing.Options{
			FilterRegistry: make(filters.Registry),
			DataClients:    []routing.DataClient{client},
			PostProcessors: []routing.PostProcessor{
				loadbalancer.NewAlgorithmProvider(),
			},
		})

		px := proxy.WithParams(proxy.Params{
			Routing: rt,
		})

		s := httptest.NewServer(px)
		p.instances = append(p.instances, &fadeInProxyInstance{
			proxy:  px,
			server: s,
		})
	}
}

func (p *fadeInProxy) endpoints() []string {
	p.mx.Lock()
	defer p.mx.Unlock()

	var ep []string
	for _, i := range p.instances {
		ep = append(ep, i.server.URL)
	}

	return ep
}

func (p *fadeInProxy) close() {
	p.mx.Lock()
	defer p.mx.Unlock()

	for _, i := range p.instances {
		i.close()
	}
}

// startClient starts a client continously polling the available proxy instances.
// The distribution of the requests across the available backend endpoints and in
// time is measured the by the client.
func startClient(test *testing.T, p *fadeInProxy) *fadeInClient {
	c := &fadeInClient{
		test:               test,
		proxy:              p,
		httpClient:         &http.Client{},
		statRequests:       make(chan statRequest),
		resetStatsRequests: make(chan struct{}),
		quit:               make(chan struct{}),
	}

	go c.run()
	return c
}

func (c *fadeInClient) run() {
	var (
		counter      int
		stats        []stat
		statRequests []statRequest
	)

	for {
		proxyEndpoints := c.proxy.endpoints()
		if len(proxyEndpoints) > 0 {
			endpoint := proxyEndpoints[counter%len(proxyEndpoints)]
			counter++

			rsp, err := c.httpClient.Get(endpoint)
			if err != nil {
				stats = append(stats, stat{
					timestamp: time.Now(),
					err:       err,
				})
			} else {
				rsp.Body.Close()
				stats = append(stats, stat{
					timestamp: time.Now(),
					status:    rsp.StatusCode,
					endpoint:  rsp.Header.Get("X-Backend-Endpoint"),
				})
			}
		}

		var pendingRequests []statRequest
		for _, sr := range statRequests {
			if len(stats) >= sr.minStats {
				sr.response <- stats
			} else {
				pendingRequests = append(pendingRequests, sr)
			}
		}

		statRequests = pendingRequests
		select {
		case sr := <-c.statRequests:
			statRequests = append(statRequests, sr)
		case <-c.resetStatsRequests:
			stats = nil
		case <-c.quit:
			return
		case <-time.After(easeClient):
		}
	}
}

func trimStartupErrors(s []stat) []stat {
	for i, si := range s {
		if si.status == http.StatusOK {
			return s[i:]
		}
	}

	return nil
}

func (c *fadeInClient) getStats(n int) []stat {
	to := time.After(statsTimeout)
	for {
		sr := statRequest{
			minStats: n,
			response: make(chan []stat, 1),
		}

		c.statRequests <- sr

		var stats []stat
		select {
		case stats = <-sr.response:
		case <-to:
			c.test.Fatal("Failed to collect stats in time.")
		}

		stats = trimStartupErrors(stats)
		if len(stats) >= minStats {
			return stats
		}

		n += minStats - len(stats)
	}
}

func (c *fadeInClient) resetStats() {
	c.resetStatsRequests <- struct{}{}
}

func (c *fadeInClient) warmUpN(n int) {
	c.getStats(n)
	c.resetStats()
}

func (c *fadeInClient) warmUp() {
	c.warmUpN(minStats)
}

func checkSuccess(t *testing.T, s []stat) {
	for _, si := range s {
		if si.status != http.StatusOK || si.endpoint == "" {
			t.Fatalf(
				"Failed request to: '%s', with status: %d.",
				si.endpoint,
				si.status,
			)
		}
	}
}

func mapStats(s []stat) map[string][]stat {
	m := make(map[string][]stat)
	for _, si := range s {
		m[si.endpoint] = append(m[si.endpoint], si)
	}

	return m
}

func statBuckets(s []stat) [][]stat {
	start := s[0].timestamp
	finish := s[len(s)-1].timestamp
	duration := finish.Sub(start)
	timeStep := duration / statBucketCount
	nextBucketStart := start.Add(timeStep)

	var (
		buckets [][]stat
		current []stat
	)

	for _, si := range s {
		for si.timestamp.After(nextBucketStart) && len(buckets) < statBucketCount-1 {
			buckets = append(buckets, current)
			current = nil
			nextBucketStart = nextBucketStart.Add(timeStep)
		}

		current = append(current, si)
	}

	buckets = append(buckets, current)
	return buckets
}

func bucketSizes(b [][]stat) []float64 {
	var sizes []float64
	for _, bi := range b {
		sizes = append(sizes, float64(len(bi)))
	}

	return sizes
}

func eqWithTolerance(left, right float64) bool {
	tolerance := float64(minStats) * epsilon / float64(statBucketCount)
	return math.Abs(left-right) < tolerance
}

func checkFadeIn(t *testing.T, s []stat) {
	buckets := statBuckets(s)
	sizes := bucketSizes(buckets)
	averageStep := sizes[len(sizes)-1] / float64(len(sizes))
	for i := range sizes {
		var prev float64
		if i > 0 {
			prev = sizes[i-1]
		}

		if !eqWithTolerance(sizes[i]-prev, averageStep) {
			t.Fatalf(
				"Unexpected fade-in step at %d. Expected: %f, got: %f.",
				i,
				averageStep,
				sizes[i]-prev,
			)
		}
	}
}

func checkStableOrDecrease(t *testing.T, s []stat) {
	buckets := statBuckets(s)
	sizes := bucketSizes(buckets)
	for i := 1; i < len(sizes); i++ {
		if sizes[i] > sizes[i-1] && !eqWithTolerance(sizes[i], sizes[i-1]) {
			t.Fatalf(
				"Unexpected increase at step %d. Expected max: %f, got: %f.",
				i,
				sizes[i-1],
				sizes[i],
			)
		}
	}
}

func (c *fadeInClient) checkFadeIn(u []string, f []bool) {
	stats := c.getStats(minStats)
	checkSuccess(c.test, stats)
	statsByEndpoint := mapStats(stats)
	for i := range u {
		if f[i] {
			checkFadeIn(c.test, statsByEndpoint[u[i]])
		} else {
			checkStableOrDecrease(c.test, statsByEndpoint[u[i]])
		}
	}
}

func (c *fadeInClient) checkNoFadeIn(u []string) {
	f := make([]bool, len(u))
	c.checkFadeIn(u, f)
}

func (c *fadeInClient) close() {
	close(c.quit)
}
