package routing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/net"
)

const defaultLastSeenTimeout = 1 * time.Minute

// Metrics describe the data about endpoint that could be
// used to perform better load balancing, fadeIn, etc.
type Metrics interface {
	DetectedTime() time.Time
	SetDetected(detected time.Time)

	LastSeen() time.Time
	SetLastSeen(lastSeen time.Time)

	InflightRequests() int64
	IncInflightRequest()
	DecInflightRequest()

	IncRequests(o IncRequestsOptions)
	HealthCheckDropProbability() float64
}

type IncRequestsOptions struct {
	FailedRoundTrip bool
}

type entry struct {
	detected         atomic.Value // time.Time
	lastSeen         atomic.Value // time.Time
	inflightRequests atomic.Int64

	totalRequests              [2]atomic.Int64
	totalFailedRoundTrips      [2]atomic.Int64
	curSlot                    atomic.Int64
	healthCheckDropProbability atomic.Value // float64
}

var _ Metrics = &entry{}

func (e *entry) DetectedTime() time.Time {
	return e.detected.Load().(time.Time)
}

func (e *entry) LastSeen() time.Time {
	return e.lastSeen.Load().(time.Time)
}

func (e *entry) InflightRequests() int64 {
	return e.inflightRequests.Load()
}

func (e *entry) IncInflightRequest() {
	e.inflightRequests.Add(1)
}

func (e *entry) DecInflightRequest() {
	e.inflightRequests.Add(-1)
}

func (e *entry) SetDetected(detected time.Time) {
	e.detected.Store(detected)
}

func (e *entry) SetLastSeen(ts time.Time) {
	e.lastSeen.Store(ts)
}

func (e *entry) IncRequests(o IncRequestsOptions) {
	curSlot := e.curSlot.Load()
	e.totalRequests[curSlot].Add(1)
	if o.FailedRoundTrip {
		e.totalFailedRoundTrips[curSlot].Add(1)
	}
}

func (e *entry) HealthCheckDropProbability() float64 {
	return e.healthCheckDropProbability.Load().(float64)
}

func newEntry() *entry {
	result := &entry{}
	result.healthCheckDropProbability.Store(0.0)
	result.SetDetected(time.Time{})
	result.SetLastSeen(time.Time{})
	return result
}

type member struct {
	endpoint string
	zone     string
}

type EndpointRegistry struct {
	lastSeenTimeout               time.Duration
	statsResetPeriod              time.Duration
	endpointsUpdatePeriod         time.Duration
	minRequests                   int64
	minHealthCheckDropProbability float64
	maxHealthCheckDropProbability float64

	quit chan struct{}

	now  func() time.Time
	data sync.Map // map[string]*entry

	endpoints       sync.Map // map[string][]*kubernetes.Endpoint
	endpointsClient *net.Client
	endpointsURL    *url.URL
}

var _ PostProcessor = &EndpointRegistry{}

type RegistryOptions struct {
	LastSeenTimeout               time.Duration
	PassiveHealthCheckEnabled     bool
	StatsResetPeriod              time.Duration
	MinRequests                   int64
	MinHealthCheckDropProbability float64
	MaxHealthCheckDropProbability float64
	ZoneAwareEnabled              bool
	EndpointsUpdatePeriod         time.Duration
	EndpointsURL                  string
}

func (r *EndpointRegistry) Do(routes []*Route) []*Route {
	now := r.now()

	for _, route := range routes {
		if route.BackendType == eskip.LBBackend {
			for i := range route.LBEndpoints {
				epi := &route.LBEndpoints[i]
				epi.Metrics = r.GetMetrics(epi.Host)
				if epi.Metrics.DetectedTime().IsZero() {
					epi.Metrics.SetDetected(now)
				}

				epi.Metrics.SetLastSeen(now)
			}
		} else if route.BackendType == eskip.NetworkBackend {
			entry := r.GetMetrics(route.Host)
			if entry.DetectedTime().IsZero() {
				entry.SetDetected(now)
			}
			entry.SetLastSeen(now)
		}
	}

	removeOlder := now.Add(-r.lastSeenTimeout)
	r.data.Range(func(key, value any) bool {
		e := value.(*entry)
		if e.LastSeen().Before(removeOlder) {
			r.data.Delete(key)
		}

		return true
	})

	return routes
}

func (r *EndpointRegistry) updateStats() {
	ticker := time.NewTicker(r.statsResetPeriod)
	defer ticker.Stop()

	for {
		r.data.Range(func(key, value any) bool {
			e := value.(*entry)

			curSlot := e.curSlot.Load()
			nextSlot := (curSlot + 1) % 2
			e.totalFailedRoundTrips[nextSlot].Store(0)
			e.totalRequests[nextSlot].Store(0)

			newDropProbability := 0.0
			failed := e.totalFailedRoundTrips[curSlot].Load()
			requests := e.totalRequests[curSlot].Load()
			if requests > r.minRequests {
				failedRoundTripsRatio := float64(failed) / float64(requests)
				if failedRoundTripsRatio > r.minHealthCheckDropProbability {
					log.Infof("Passive health check: marking %q as unhealthy due to failed round trips ratio: %0.2f", key, failedRoundTripsRatio)
					newDropProbability = min(failedRoundTripsRatio, r.maxHealthCheckDropProbability)
				}
			}
			e.healthCheckDropProbability.Store(newDropProbability)
			e.curSlot.Store(nextSlot)

			return true
		})

		select {
		case <-r.quit:
			return
		case <-ticker.C:
		}
	}
}

type EndpointsMap map[string][]*Endpoint

type Endpoint struct {
	Address string `json:"addr"`
	Zone    string `json:"zone"`
	Port    int    `json:"port"`
}

func (r *EndpointRegistry) fetchEndpoints() (EndpointsMap, error) {

	req, err := http.NewRequest("GET", r.endpointsURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch endpoints request: %w", err)
	}

	rsp, err := r.endpointsClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get fetch endpoints: %w", err)
	}
	defer rsp.Body.Close()

	// TODO(sszuecs): whatever we have in routesrv as API response we need to adapt here
	buf, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	log.Debugf("Go %s", buf)

	var result EndpointsMap
	err = json.Unmarshal(buf, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal into kubernetes.EndpointsMap: %w", err)
	}
	return result, nil
}

func (r *EndpointRegistry) updateEndpoints() {
	ticker := time.NewTicker(r.endpointsUpdatePeriod)
	defer ticker.Stop()

	for {
		mapMembers, err := r.fetchEndpoints()
		if err != nil {
			log.Errorf("Failed to fetch endpoints: %v", err)
		} else {
			active := make(map[string]struct{})
			log.Infof("Got %d members", len(mapMembers))

			// create/update
			for svc, members := range mapMembers {
				r.endpoints.Store(svc, members)
				active[svc] = struct{}{}
				for _, member := range members {
					println("service:", svc, "with addr:", member.Address, "in zone", member.Zone, "with port", member.Port)
				}
			}

			// cleanup endpoints
			r.endpoints.Range(func(a, _ any) bool {
				svc := a.(string)
				if _, ok := active[svc]; !ok {
					r.endpoints.Delete(svc)
				}
				return true
			})
		}

		select {
		case <-r.quit:
			return
		case <-ticker.C:
		}
	}
}

func (r *EndpointRegistry) Close() {
	close(r.quit)
}

func NewEndpointRegistry(o RegistryOptions) *EndpointRegistry {
	if o.LastSeenTimeout == 0 {
		o.LastSeenTimeout = defaultLastSeenTimeout
	}

	var epURL *url.URL
	if u, err := url.Parse(o.EndpointsURL); err != nil {
		log.Errorf("Failed to parse %q: %v", o.EndpointsURL, err)
		log.Infof("Disable zone awareness endpoint updating")
		o.ZoneAwareEnabled = false
	} else {
		epURL = u
	}

	registry := &EndpointRegistry{
		lastSeenTimeout:               o.LastSeenTimeout,
		statsResetPeriod:              o.StatsResetPeriod,
		minRequests:                   o.MinRequests,
		minHealthCheckDropProbability: o.MinHealthCheckDropProbability,
		maxHealthCheckDropProbability: o.MaxHealthCheckDropProbability,
		endpointsURL:                  epURL,
		endpointsUpdatePeriod:         5 * time.Second,

		quit: make(chan struct{}),

		now:  time.Now,
		data: sync.Map{},
	}
	if o.PassiveHealthCheckEnabled {
		go registry.updateStats()
	}
	if o.ZoneAwareEnabled {
		registry.endpointsClient = net.NewClient(net.Options{
			MaxIdleConnsPerHost: 3,
			MaxIdleConns:        10,
			IdleConnTimeout:     10 * time.Second,
		})

		go registry.updateEndpoints()
	}

	return registry
}

func (r *EndpointRegistry) GetMetrics(hostPort string) Metrics {
	// https://github.com/golang/go/issues/44159#issuecomment-780774977
	e, ok := r.data.Load(hostPort)
	if !ok {
		e, _ = r.data.LoadOrStore(hostPort, newEntry())
	}
	return e.(*entry)
}

func (r *EndpointRegistry) allMetrics() map[string]Metrics {
	result := make(map[string]Metrics)
	r.data.Range(func(k, v any) bool {
		result[k.(string)] = v.(Metrics)
		return true
	})
	return result
}
