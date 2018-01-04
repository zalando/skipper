// Package loadbalancer implements health checking of pool mmebers for
// a group of routes.
//
// Based on https://landing.google.com/sre/book/chapters/load-balancing-datacenter.html#identifying-bad-tasks-flow-control-and-lame-ducks-bEs0uy we use
//
// Healthy (healthy)
//
//     The backend task has initialized correctly and is processing
//     requests.
//
// Refusing connections (dead)
//
//     The backend task is unresponsive. This can happen because the
//     task is starting up or shutting down, or because the backend is
//     in an abnormal state (though it would be rare for a backend to
//     stop listening on its port if it is not shutting down).
//
// Lame duck (unhealthy)
//
//     The backend task is listening on its port and can serve, but is
//     explicitly asking clients to stop sending requests. This is
//     also used in case of SIGTERM to graceful shutdown pool
//     membership
package loadbalancer

import (
	"bytes"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/mailgun/log"
	"github.com/zalando/skipper/eskip"
)

type HealthChecker struct {
	sync.RWMutex
	pools group
}

type group map[string]*pool

type pool struct {
	member         []member
	healthEndpoint string // for example /healthz
}

type member struct {
	mu    *sync.RWMutex
	route *eskip.Route
	state state
}

type state int

const (
	pending   state = 1 << iota
	healthy         // pool member serving traffic and accepting new connections
	unhealthy       // pool member probably serving traffic but should not get new connections, because of SIGTERM, overload, ..
	dead            // pool member we can not TCP connect to and net.Error#Temporary() returns false, this state should be considered safe for retry another backend
)

func NewHealthChecker() *HealthChecker {
	g := make(group, 1)
	return &HealthChecker{
		pools: g,
	}
}

// StartActiveHealthChecker does active health checking of all pool
// members based on healthEndpoint of the pool.
func (hc *HealthChecker) StartActiveHealthChecker(d time.Duration, quitCH chan struct{}) error {
	if hc == nil {
		return nil
	}
	ticker := time.NewTicker(d)
	go func() {
		rt := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   500 * time.Millisecond,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			TLSHandshakeTimeout:   500 * time.Millisecond,
			ResponseHeaderTimeout: 1 * time.Second,
			ExpectContinueTimeout: 500 * time.Millisecond,
			MaxIdleConns:          20, // 0 -> no limit
			MaxIdleConnsPerHost:   1,  // http.DefaultMaxIdleConnsPerHost=2
			IdleConnTimeout:       10 * time.Second,
		}

		for {
			select {
			case t := <-ticker.C:
				log.Debugf("Tick at", t)
				now := time.Now()
				hc.Lock()
				for g, p := range hc.pools {
					log.Debugf("Check %s with %d members", g, len(p.member))
					go checkActiveHealth(rt, g, p.healthEndpoint, p.member)
				}
				hc.Unlock()
				log.Debugf("Took %v", time.Now().Sub(now))
			case <-quitCH:
				ticker.Stop()
			}
		}
	}()
	return nil
}

// checkActiveHealth sets state of each member according to the result
// of the call to healthEndpoint of each member.
func checkActiveHealth(rt http.RoundTripper, group, healthEndpoint string, members []member) {
	for _, member := range members {
		// TODO(sszuecs): check if we need to lock member in case PRE: member is only in one pool
		member.mu.RLock()
		backendType := member.route.BackendType
		backend := member.route.Backend
		formerState := member.state
		member.mu.RUnlock()

		s := doActiveHealthCheckOld(rt, healthEndpoint, backend, backendType, formerState)
		member.mu.Lock()
		member.state = s
		member.mu.Unlock()
	}
}

func doActiveHealthCheckOld(rt http.RoundTripper, healthEndpoint, backend string, backendType eskip.BackendType, formerState state) state {
	if backendType != eskip.NetworkBackend {
		return healthy
	}

	u, err := url.Parse(backend)
	if err != nil {
		log.Errorf("Failed to parse route backend %s: %v", backend, err)
		return formerState
	}

	buf := make([]byte, 1024)
	req, err := http.NewRequest("GET", u.String(), bytes.NewReader(buf))
	if err != nil {
		log.Errorf("Failed to create health check request: %v", err)
		return formerState
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		perr, ok := err.(net.Error)
		if ok && !perr.Temporary() {
			log.Infof("Backend %v connection refused -> mark as dead", backend)
			return dead
		} else if ok {
			log.Infof("Backend %v with temporary error '%v' -> mark as unhealthy", backend, perr)
			return unhealthy
		}
		log.Errorf("Failed to do health check %v", err)
		return formerState
	}

	// we only check StatusCode
	resp.Body.Close()

	switch code := resp.StatusCode; code {
	case http.StatusOK:
		return healthy
	default:
		return unhealthy
	}
}

// HealthyMemberRoutes filters pool members of the given group by
// healthy state.
func (hc *HealthChecker) HealthyMemberRoutes(group string) []*eskip.Route {
	if hc == nil {
		return nil
	}

	results := make([]*eskip.Route, 0)
	hc.RLock()
	for _, m := range hc.pools[group].member {
		m.mu.RLock()
		if m.state == healthy {
			results = append(results, m.route)
		}
		m.mu.RUnlock()
	}
	hc.RUnlock()
	return results
}

// ContainsRoute returns true if HealthChecker is nil or the route.Id
// is found in the given group of members, otherwise false.
func (hc *HealthChecker) ContainsRoute(group string, route *eskip.Route) bool {
	return hc.findIndex(group, route) != -1
}

// Delete given route for given pool
func (hc *HealthChecker) Delete(group string, route *eskip.Route) {
	if hc == nil {
		return
	}

	idx := hc.findIndex(group, route)
	if idx >= 0 {
		hc.Lock()
		hc.pools[group].member = append(hc.pools[group].member[:idx], hc.pools[group].member[idx+1:]...)
		hc.Unlock()
	}
}

func (hc *HealthChecker) findIndex(group string, r *eskip.Route) int {
	if hc == nil {
		return -1
	}

	idx := -1

	hc.RLock()
	for i, m := range hc.pools[group].member {
		if m.route.Id == r.Id {
			idx = i
			break
		}
	}
	hc.RUnlock()

	return idx
}

// Upsert will add a member only if it does not exist, yet.
func (hc *HealthChecker) Upsert(group string, r *eskip.Route) {
	if hc == nil {
		return
	}

	if !hc.ContainsRoute(group, r) {
		hc.add(group, r)
	}
}

// Add a new member route with state pending to a pool identified by group.
// TODO(sszuecs): we have to make sure we do not add the same member with state pending all the time.
func (hc *HealthChecker) add(group string, newRoute *eskip.Route) {
	if hc == nil {
		return
	}

	newMember := member{
		mu:    &sync.RWMutex{},
		route: newRoute,
		state: pending,
	}
	hc.Lock()
	hc.pools[group].member = append(hc.pools[group].member, newMember)
	hc.Unlock()
}

// NEW implementation - not using all the things above
type LB struct {
	ch     chan *eskip.Route
	quitCH chan struct{}
}

func NewLB(quitCH chan struct{}) *LB {
	return &LB{
		ch:     make(chan *eskip.Route, 100),
		quitCH: quitCH,
	}
}

func (lb *LB) HealthyMemberRoutes(routes []*eskip.Route) []*eskip.Route {
	result := make([]*eskip.Route, 0)
	for _, r := range routes {
		if r.Group != "" {
			switch r.State {
			case eskip.Pending:
				log.Infof("filtered pending member route and schedule health check: %v", r)
				lb.ch <- r
				continue
			case eskip.Healthy:
				// pass
			case eskip.Unhealthy:
				log.Infof("filtered unhealthy member route and scheduled health check: %v", r)
				lb.scheduleCheckWithDelay(r, time.Second*3)
				continue
			case eskip.Dead:
				log.Infof("filtered dead member route and scheduled health check: %v", r)
				lb.scheduleCheckWithDelay(r, time.Second*30)
				continue
			}
		}
		result = append(result, r)
	}
	return result
}

func (lb *LB) scheduleCheckWithDelay(route *eskip.Route, d time.Duration) {
	go func(route *eskip.Route, d time.Duration) {
		time.Sleep(d)
		lb.ch <- route
	}(route, d)
}

func (lb *LB) StartDoHealthCheck(d time.Duration) {
	ticker := time.NewTicker(d)
	go func() {
		rt := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   500 * time.Millisecond,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			TLSHandshakeTimeout:   500 * time.Millisecond,
			ResponseHeaderTimeout: 1 * time.Second,
			ExpectContinueTimeout: 500 * time.Millisecond,
			MaxIdleConns:          20, // 0 -> no limit
			MaxIdleConnsPerHost:   1,  // http.DefaultMaxIdleConnsPerHost=2
			IdleConnTimeout:       10 * time.Second,
		}

		for {
			select {
			case t := <-ticker.C:
				log.Debugf("Tick at", t)
				now := time.Now()
				for r := range lb.ch {
					s := doActiveHealthCheck(rt, "healthEndpoint", r)
					r.State = s
				}
				log.Debugf("Took %v", time.Now().Sub(now))
			case <-lb.quitCH:
				ticker.Stop()
			}
		}
	}()
}

func doActiveHealthCheck(rt http.RoundTripper, healthEndpoint string, route *eskip.Route) eskip.LBState {
	if route.BackendType != eskip.NetworkBackend {
		return eskip.Healthy
	}

	u, err := url.Parse(route.Backend)
	if err != nil {
		log.Errorf("Failed to parse route backend %s: %v", route.Backend, err)
		return route.State
	}

	buf := make([]byte, 1024)
	req, err := http.NewRequest("GET", u.String(), bytes.NewReader(buf))
	if err != nil {
		log.Errorf("Failed to create health check request: %v", err)
		return route.State
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		perr, ok := err.(net.Error)
		if ok && !perr.Temporary() {
			log.Infof("Backend %v connection refused -> mark as dead", route.Backend)
			return eskip.Dead
		} else if ok {
			log.Infof("Backend %v with temporary error '%v' -> mark as unhealthy", route.Backend, perr)
			return eskip.Unhealthy
		}
		log.Errorf("Failed to do health check %v", err)
		return route.State
	}

	// we only check StatusCode
	resp.Body.Close()

	switch code := resp.StatusCode; code {
	case http.StatusOK:
		return eskip.Healthy
	default:
		return eskip.Unhealthy
	}
}
