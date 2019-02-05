package loadbalancer

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

type state int

// TODO:
// - can these states really combined or it's just an incremental enum?
// - shouldn't `unknown` be 0?
const (
	healthy   state = 1 << iota // pool member serving traffic and accepting new connections
	unhealthy                   // pool member probably serving traffic but should not get new connections, because of SIGTERM, overload, ..
	dead                        // pool member we can not TCP connect to and net.Error#Temporary() returns false, this state should be considered safe for retry another backend
	unknown                     // could not be checked by our health checker -> do not change state
)

// LB stores state of routes, which were reported dead or unhealthy by
// other packages, f.e. proxy.  Based on reported routes LB starts to
// do active healthchecks to find if a route becomes haelthy again. Use NewLB() to create an LB
type LB struct {
	sync.RWMutex
	ch                  chan string
	sigtermSignal       chan os.Signal
	stop                bool
	healthcheckInterval time.Duration
	routeState          map[string]state
}

// HealthcheckPostProcessor wraps the LB structure implementing the
// routing.PostProcessor interface for filtering healthy routes.
type HealthcheckPostProcessor struct{ *LB }

// Do filters the routes with healthy backends.
func (hcpp HealthcheckPostProcessor) Do(r []*routing.Route) []*routing.Route {
	return hcpp.LB.FilterHealthyMemberRoutes(r)
}

// NewLB creates a new LB and starts background jobs for populating
// backends to check added routes and checking them every
// healthcheckInterval.
func New(healthcheckInterval time.Duration) *LB {
	if healthcheckInterval == 0 {
		return nil
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)

	lb := &LB{
		ch:                  make(chan string, 100), // buffered channel to be not a blocking call to write to
		sigtermSignal:       sigs,
		stop:                false,
		healthcheckInterval: healthcheckInterval,
		routeState:          make(map[string]state),
	}
	go lb.populateChecks()
	go lb.startDoHealthChecks()
	return lb
}

func (lb *LB) populateChecks() {
	for s := range lb.ch {
		if lb.stop {
			return
		}
		lb.Lock()
		if st, ok := lb.routeState[s]; !ok || st == healthy {
			lb.routeState[s] = unhealthy
		}
		lb.Unlock()
	}
}

// AddHealthcheck can be used to report unhealthy routes, which
// loadbalancer will use to do active healthchecking and dataclients
// can ask the loadbalancer to filter unhealhyt or dead routes.
func (lb *LB) AddHealthcheck(backend string) {
	if lb == nil || lb.stop {
		return
	}
	log.Infof("add backend to be health checked by the loadbalancer: %s", backend)
	lb.ch <- backend
}

// FilterHealthyMemberRoutes can be used by dataclients to filter for
// routes that have known not healthy backends.
func (lb *LB) FilterHealthyMemberRoutes(routes []*routing.Route) []*routing.Route {
	// NOTE: it would be awesome to add a logic, that cleans off the unhealthy endpoints or triggers it.
	// For that we'll add some interface, so that different scenarios can benefit from it, but I think it's
	// still not the eskip level is the right one for that.

	if lb == nil {
		return routes
	}
	var result []*routing.Route
	knownBackends := make(map[string]bool)
	for _, r := range routes {
		knownBackends[r.Backend] = true
		if r.BackendType == eskip.LBBackend {
			var st state
			lb.RLock()
			st, ok := lb.routeState[r.Backend]
			lb.RUnlock()
			if ok {
				switch st {
				case unhealthy:
					fallthrough
				case dead:
					log.Infof("filtered member route: %v", r)
					continue
				}
			}
		}
		result = append(result, r)
	}

	lb.Lock()
	for b := range lb.routeState {
		if _, ok := knownBackends[b]; !ok {
			delete(lb.routeState, b)
		}
	}
	lb.Unlock()

	log.Infof("filterRoutes incoming=%d outgoing=%d", len(routes), len(result))
	return result
}

// startDoHealthChecks will schedule every healthcheckInterval
// healthchecks to all backends, which were reported.
func (lb *LB) startDoHealthChecks() {
	healthTicker := time.NewTicker(lb.healthcheckInterval)
	rt := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   3000 * time.Millisecond,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		TLSHandshakeTimeout:   3000 * time.Millisecond,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 5000 * time.Millisecond,
		MaxIdleConns:          20, // 0 -> no limit
		MaxIdleConnsPerHost:   1,  // http.DefaultMaxIdleConnsPerHost=2
		IdleConnTimeout:       10 * time.Second,
	}

	for {
		select {
		case <-healthTicker.C:
			now := time.Now()

			lb.RLock()
			backends := make([]string, 0, len(lb.routeState))
			for b := range lb.routeState {
				backends = append(backends, b)
			}
			lb.RUnlock()

			for _, backend := range backends {
				st := doActiveHealthCheck(rt, backend)
				switch st {
				case unknown:
					continue
				case healthy:
					lb.Lock()
					delete(lb.routeState, backend)
					lb.Unlock()
				default:
					lb.Lock()
					lb.routeState[backend] = st
					lb.Unlock()
				}
			}
			log.Debugf("Checking health took %v", time.Since(now))

		case <-lb.sigtermSignal:
			log.Infoln("Shutdown loadbalancer")
			healthTicker.Stop()
			lb.stop = true
			close(lb.ch)
			return
		}
	}
}

func doActiveHealthCheck(rt http.RoundTripper, backend string) state {
	u, err := url.Parse(backend)
	if err != nil {
		log.Errorf("Failed to parse route backend %s: %v", backend, err)
		return unknown
	}

	buf := make([]byte, 128)
	req, err := http.NewRequest("GET", u.String(), bytes.NewReader(buf))
	if err != nil {
		log.Errorf("Failed to create health check request: %v", err)
		return unknown
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
		log.Errorf("Failed to do health check, but no network error: %v", err)
		return unknown
	}

	// we only check StatusCode
	// TODO: check StatusCode ;)
	io.Copy(ioutil.Discard, resp.Body)
	resp.Body.Close()

	log.Infof("Backend %v is healthy again", backend)
	return healthy
}
