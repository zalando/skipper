// Package loadbalancer implements health checking of pool members for
// a group of routes, if backend calls are reported to the loadbalancer.
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
//     explicitly asking clients to stop sending requests.
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

	"github.com/mailgun/log"
	"github.com/zalando/skipper/eskip"
)

type state int

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

// NewLB creates a new LB and starts background jobs for populating
// backends to check added routes and checking them every
// healthcheckInterval.
func NewLB(healthcheckInterval time.Duration) *LB {
	if healthcheckInterval == 0 {
		return nil
	}
	var sigs chan os.Signal
	sigs = make(chan os.Signal, 1)
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
		lb.routeState[s] = unhealthy
		lb.Unlock()
	}
}

// AddHealthcheck can be used to report unhealty routes, which
// loadbalancer will use to do active healthchecking and dataclients
// can ask the loadbalancer to filter unhealhyt or dead routes.
func (lb *LB) AddHealthcheck(backend string) {
	if lb == nil {
		return
	}
	lb.ch <- backend
}

// FilterHealthyMemberRoutes can be used by dataclients to filter for
// routes that have known not healthy backends.
func (lb *LB) FilterHealthyMemberRoutes(routes []*eskip.Route) []*eskip.Route {
	if lb == nil {
		return routes
	}
	result := make([]*eskip.Route, 0)
	for _, r := range routes {
		if r.Group != "" { // only loadbalanced routes have a Group, set by dataclients
			lb.RLock()
			st, ok := lb.routeState[r.Backend]
			lb.RUnlock()
			if ok {
				switch st {
				case dead:
					log.Infof("filtered member route: %v", r)
					continue
				}
			}
		}
		result = append(result, r)
	}
	return result
}

// startDoHealthChecks will schedule every healthcheckInterval
// healthchecks to all backends, which were reported.
func (lb *LB) startDoHealthChecks() {
	ticker := time.NewTicker(lb.healthcheckInterval)
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

			lb.RLock()
			backends := make([]string, len(lb.routeState))
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
			log.Debugf("Took %v", time.Now().Sub(now))
		case <-lb.sigtermSignal:
			ticker.Stop()
			lb.stop = true
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
	io.Copy(ioutil.Discard, resp.Body)
	resp.Body.Close()

	// If we are here we imagine the owner of the application does it right
	return healthy
}
