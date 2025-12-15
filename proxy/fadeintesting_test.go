package proxy_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/fadein"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

/*
Creating a separate test harness for the fade-in functionality, because the existing test proxy doesn't support
multiple proxy instances and should be wrapped anyway, while the changes in the routing in these tests are
consequences to events happening to the backend instances, and therefore the test data client should be wrapped,
too. The harness implements the following setup:

	client -> proxy -> data client <- backend
	          |-> proxy instance 1    |-> backend instance 1
	          |-> proxy instance 2    |-> backend instance 2
	          \-> proxy instance 3    |-> backend instance 3
	                                  \-> backend instance 4
*/

const (
	testFadeInDuration = 6000 * time.Millisecond
	statBucketCount    = 10
	clientRate         = time.Millisecond
	minStats           = 300
	fadeInTolerance    = 0.3
)

type fadeInDataClient struct {
	reset, update chan []*eskip.Route
	quit          chan struct{}
}

type fadeInBackendInstance struct {
	server  *httptest.Server
	created time.Time
}

type fadeInBackend struct {
	test        *testing.T
	withFadeIn  bool
	withCreated bool
	clients     []fadeInDataClient
	instances   []fadeInBackendInstance
}

type fadeInProxyInstance struct {
	routing *routing.Routing
	proxy   *proxy.Proxy
	server  *httptest.Server
}

type fadeInProxy struct {
	test      *testing.T
	mu        sync.Mutex
	backend   *fadeInBackend
	instances []*fadeInProxyInstance
}

type stat struct {
	timestamp time.Time
	status    int
	endpoint  string
	err       error
}

type fadeInClient struct {
	test               *testing.T
	proxy              *fadeInProxy
	httpClient         *http.Client
	statRequests       chan chan []stat
	resetStatsRequests chan struct{}
	quit               chan struct{}
}

func randomURLs(t *testing.T, n int) []string {
	var u []string
	for range n {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}

		_, p, err := net.SplitHostPort(l.Addr().String())
		if err != nil {
			t.Fatal(err)
		}

		l.Close()
		u = append(u, fmt.Sprintf(":%s", p))
	}

	return u
}

func createDataClient(r ...*eskip.Route) fadeInDataClient {
	var c fadeInDataClient
	c.reset = make(chan []*eskip.Route, 1)
	c.update = make(chan []*eskip.Route, 1)
	c.quit = make(chan struct{})
	c.reset <- r
	return c
}

func (c fadeInDataClient) LoadAll() ([]*eskip.Route, error) {
	select {
	case r := <-c.reset:
		c.reset <- r
		return r, nil
	case <-c.quit:
		return nil, nil
	}
}

func (c fadeInDataClient) LoadUpdate() ([]*eskip.Route, []string, error) {
	select {
	// hm, blocking dataclient?
	case r := <-c.update:
		return r, nil, nil
	case <-c.quit:
		return nil, nil, nil
	}
}

func (c fadeInDataClient) close() {
	close(c.quit)
}

// startBackend starts a backend representing 0 or more endpoints, added in a separate step.
func startBackend(t *testing.T, withFadeIn, withCreated bool) *fadeInBackend {
	return &fadeInBackend{
		test:        t,
		withFadeIn:  withFadeIn,
		withCreated: withCreated,
	}
}

func (b *fadeInBackend) route() *eskip.Route {
	r := &eskip.Route{
		Id:          "fadeInRoute",
		BackendType: eskip.LBBackend,
	}

	if b.withFadeIn {
		r.Filters = append(r.Filters, &eskip.Filter{
			Name: filters.FadeInName,
			Args: []interface{}{testFadeInDuration},
		})
	}

	for _, i := range b.instances {
		r.LBEndpoints = append(r.LBEndpoints, i.server.URL)
		if !i.created.IsZero() {
			r.Filters = append(r.Filters, &eskip.Filter{
				Name: filters.EndpointCreatedName,
				Args: []interface{}{
					i.server.URL,
					i.created,
				},
			})
		}
	}

	return r
}

func (b *fadeInBackend) createDataClient() routing.DataClient {
	c := createDataClient(b.route())
	b.clients = append(b.clients, c)
	return c
}

func (b *fadeInBackend) addInstances(u ...string) error {
	var instances []fadeInBackendInstance
	for _, ui := range u {
		if err := func(u string) error {
			l, err := net.Listen("tcp", u)
			if err != nil {
				return err
			}

			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Backend-Endpoint", u)
			}))
			server.Listener.Close()
			server.Listener = l
			server.Start()
			instance := fadeInBackendInstance{
				server: server,
			}

			if b.withCreated {
				instance.created = time.Now()
			}

			instances = append(instances, instance)
			return nil
		}(ui); err != nil {
			for _, i := range instances {
				i.server.Close()
			}

			return err
		}
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

	return nil
}

func (b *fadeInBackend) close() {
	for _, i := range b.instances {
		i.server.Close()
	}
	for _, c := range b.clients {
		c.close()
	}
}

func (p *fadeInProxyInstance) close() {
	p.server.Close()
	p.proxy.Close()
	p.routing.Close()
}

// startProxy starts a proxy representing 0 or more proxy instances, added in a separate step.
func startProxy(t *testing.T, b *fadeInBackend) *fadeInProxy {
	return &fadeInProxy{
		test:    t,
		backend: b,
	}
}

func (p *fadeInProxy) addInstances(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := 0; i < n; i++ {
		client := p.backend.createDataClient()
		fr := make(filters.Registry)
		fr.Register(fadein.NewFadeIn())
		fr.Register(fadein.NewEndpointCreated())

		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		rt := routing.New(routing.Options{
			FilterRegistry: fr,
			DataClients:    []routing.DataClient{client},
			PostProcessors: []routing.PostProcessor{
				loadbalancer.NewAlgorithmProvider(),
				endpointRegistry,
				fadein.NewPostProcessor(fadein.PostProcessorOptions{EndpointRegistry: endpointRegistry}),
			},
		})

		px := proxy.WithParams(proxy.Params{
			Routing: rt,
		})

		s := httptest.NewServer(px)
		p.instances = append(p.instances, &fadeInProxyInstance{
			routing: rt,
			proxy:   px,
			server:  s,
		})
	}
}

func (p *fadeInProxy) endpoints() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	var ep []string
	for _, i := range p.instances {
		ep = append(ep, i.server.URL)
	}

	return ep
}

func (p *fadeInProxy) close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, i := range p.instances {
		i.close()
	}
}

// startClient starts a client continuously polling the available proxy instances.
// The distribution of the requests across the available backend endpoints and in
// time is measured the by the client.
func startClient(test *testing.T, p *fadeInProxy) *fadeInClient {
	c := &fadeInClient{
		test:               test,
		proxy:              p,
		httpClient:         &http.Client{},
		statRequests:       make(chan chan []stat),
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
		statRequests []chan []stat
	)

	for {
		proxyEndpoints := c.proxy.endpoints()
		var requestStart time.Time
		if len(proxyEndpoints) > 0 {
			endpoint := proxyEndpoints[counter%len(proxyEndpoints)]
			counter++

			requestStart = time.Now()
			if rsp, err := c.httpClient.Get(endpoint); err != nil {
				stats = append(stats, stat{
					timestamp: requestStart,
					err:       err,
				})
			} else {
				rsp.Body.Close()
				stats = append(stats, stat{
					timestamp: requestStart,
					status:    rsp.StatusCode,
					endpoint:  rsp.Header.Get("X-Backend-Endpoint"),
				})
			}
		}

		for _, sr := range statRequests {
			sr <- stats
		}

		var nextRequest <-chan time.Time
		if !requestStart.IsZero() {
			nextRequest = time.After(clientRate - time.Since(requestStart))
		} else {
			nextRequest = time.After(clientRate)
		}

		statRequests = nil
		select {
		case sr := <-c.statRequests:
			statRequests = append(statRequests, sr)
		case <-c.resetStatsRequests:
			stats = nil
		case <-c.quit:
			return
		case <-nextRequest:
		}
	}
}

func (c *fadeInClient) getStats() []stat {
	ch := make(chan []stat, 1)
	c.statRequests <- ch
	return <-ch
}

func (c *fadeInClient) resetStats() {
	c.resetStatsRequests <- struct{}{}
}

func (c *fadeInClient) warmUpD(d time.Duration) {
	time.Sleep(d)
	c.resetStats()
}

func (c *fadeInClient) warmUp() {
	c.warmUpD(testFadeInDuration)
}

func (c *fadeInClient) close() {
	close(c.quit)
}

func trimFailed(s []stat) []stat {
	for i := range s {
		if s[i].status >= 200 && s[i].status < 300 {
			return s[i:]
		}
	}

	return nil
}

func checkSuccess(t *testing.T, s []stat) {
	var foundAny bool
	for _, si := range s {
		foundAny = true
		if si.status != http.StatusOK || si.endpoint == "" {
			t.Fatalf(
				"Failed request to: '%s', with status: %d.",
				si.endpoint,
				si.status,
			)
		}
	}

	if !foundAny {
		t.Fatal("Failed to generate stats.")
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

func checkSamples(t *testing.T, s []stat) {
	if len(s) < minStats {
		t.Fatalf("No sufficient stats: %d, expected at least: %d.", len(s), minStats)
	}
}

func checkEndpointFadeIn(t *testing.T, s []stat) {
	checkSamples(t, s)
	buckets := statBuckets(s)
	sizes := bucketSizes(buckets)
	if sizes[0] >= sizes[len(sizes)/2] || sizes[len(sizes)/2] >= sizes[len(sizes)-1] {
		t.Fatal("Failed to fade-in.")
	}
}

func lessOrEqWithTolerance(left, right float64) bool {
	t := (left + right) * fadeInTolerance / 2
	return left < right+t
}

func checkStableOrDecrease(t *testing.T, s []stat) {
	checkSamples(t, s)
	buckets := statBuckets(s)
	sizes := bucketSizes(buckets)
	if !lessOrEqWithTolerance(sizes[len(sizes)/2], sizes[0]) ||
		!lessOrEqWithTolerance(sizes[len(sizes)-1], sizes[len(sizes)/2]) {
		t.Fatal("Unexpected increase.")
	}
}

func checkFadeIn(t *testing.T, u []string, stats []stat, doFade []bool) {
	checkSuccess(t, stats)
	statsByEndpoint := mapStats(stats)
	for i := range u {
		if i < len(doFade) && doFade[i] {
			checkEndpointFadeIn(t, statsByEndpoint[u[i]])
		} else if i < len(doFade) {
			checkStableOrDecrease(t, statsByEndpoint[u[i]])
		}
	}
}

func endpointStartTest(
	proxies, initialEndpoints, addEndpoints int,
	withFadeIn, withCreated bool,
	expectFadeIn ...bool,
) func(*testing.T) {
	return func(t *testing.T) {
		b := startBackend(t, withFadeIn, withCreated)
		defer b.close()

		initial := randomURLs(t, initialEndpoints)
		if err := b.addInstances(initial...); err != nil {
			t.Fatal(err)
		}

		p := startProxy(t, b)
		defer p.close()
		p.addInstances(proxies)

		c := startClient(t, p)
		defer c.close()
		c.warmUp()

		add := randomURLs(t, addEndpoints)
		b.addInstances(add...)

		time.Sleep(testFadeInDuration)
		stats := c.getStats()
		stats = trimFailed(stats)
		checkFadeIn(t, append(initial, add...), stats, expectFadeIn)
	}
}

func sub(title string, tests ...func(*testing.T)) func(*testing.T) {
	return func(t *testing.T) {
		t.Run(title, func(t *testing.T) {
			t.Parallel()
			for _, test := range tests {
				test(t)
			}
		})
	}
}

func run(t *testing.T, title string, tests ...func(*testing.T)) {
	sub(title, tests...)(t)
}
