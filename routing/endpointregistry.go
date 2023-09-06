package routing

import (
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

const lastSeenTimeout = 1 * time.Minute

const (
	// Deprecated, use filters.FadeInName instead
	FadeInName = filters.FadeInName
	// Deprecated, use filters.EndpointCreatedName instead
	EndpointCreatedName = filters.EndpointCreatedName
)

type (
	fadeIn struct {
		duration time.Duration
		exponent float64
	}

	endpointCreated struct {
		when  time.Time
		which string
	}
)

// NewFadeIn creates a filter spec for the fade-in filter.
func NewFadeIn() filters.Spec {
	return fadeIn{}
}

func (fadeIn) Name() string { return filters.FadeInName }

func (fadeIn) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var f fadeIn
	switch v := args[0].(type) {
	case int:
		f.duration = time.Duration(v * int(time.Millisecond))
	case float64:
		f.duration = time.Duration(v * float64(time.Millisecond))
	case string:
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, err
		}

		f.duration = d
	case time.Duration:
		f.duration = v
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	f.exponent = 1
	if len(args) == 2 {
		switch v := args[1].(type) {
		case int:
			f.exponent = float64(v)
		case float64:
			f.exponent = v
		default:
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return f, nil
}

func (fadeIn) Request(filters.FilterContext)  {}
func (fadeIn) Response(filters.FilterContext) {}

// NewEndpointCreated creates a filter spec for the endpointCreated filter.
func NewEndpointCreated() filters.Spec {
	var ec endpointCreated
	return ec
}

func (endpointCreated) Name() string { return filters.EndpointCreatedName }

func normalizeSchemeHost(s, h string) (string, string, error) {
	// endpoint address cannot contain path, the rest is not case sensitive
	s, h = strings.ToLower(s), strings.ToLower(h)

	hh, p, err := net.SplitHostPort(h)
	if err != nil {
		// what is the actual right way of doing this, considering IPv6 addresses, too?
		if !strings.Contains(err.Error(), "missing port") {
			return "", "", err
		}

		p = ""
	} else {
		h = hh
	}

	switch {
	case p == "" && s == "http":
		p = "80"
	case p == "" && s == "https":
		p = "443"
	}

	h = net.JoinHostPort(h, p)
	return s, h, nil
}

func normalizeEndpoint(e string) (string, string, error) {
	u, err := url.Parse(e)
	if err != nil {
		return "", "", err
	}

	return normalizeSchemeHost(u.Scheme, u.Host)
}

func (endpointCreated) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	e, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	_, h, err := normalizeEndpoint(e)
	if err != nil {
		return nil, err
	}

	ec := endpointCreated{which: h}
	switch v := args[1].(type) {
	case int:
		ec.when = time.Unix(int64(v), 0)
	case float64:
		ec.when = time.Unix(int64(v), 0)
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, err
		}

		ec.when = t
	case time.Time:
		ec.when = v
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	// mitigate potential flakiness caused by clock skew. When the created time is in the future based on
	// the local clock, we ignore it.
	now := time.Now()
	if ec.when.After(now) {
		log.Errorf(
			"Endpoint created time in the future, fading in without endpoint creation time: %v. Potential clock skew.",
			ec.when,
		)

		ec.when = time.Time{}
	}

	return ec, nil
}

func (endpointCreated) Request(filters.FilterContext)  {}
func (endpointCreated) Response(filters.FilterContext) {}

// Metrics describe the data about endpoint that could be
// used to perform better load balancing, fadeIn, etc.
type Metrics interface {
	DetectedTime() time.Time
	InflightRequests() int64
}

type entry struct {
	detected         time.Time
	lastActive       time.Time
	fadeInDuration   time.Duration
	inflightRequests int64
}

var _ Metrics = &entry{}

func (e *entry) DetectedTime() time.Time {
	return e.detected
}

func (e *entry) InflightRequests() int64 {
	return e.inflightRequests
}

type EndpointRegistry struct {
	lastSeen map[string]time.Time
	now      func() time.Time

	mu sync.Mutex

	data map[string]*entry
}

var _ PostProcessor = &EndpointRegistry{}

type RegistryOptions struct {
}

func (r *EndpointRegistry) Do(routes []*Route) []*Route {
	now := r.now()
	for _, route := range routes {
		if route.Route.BackendType != eskip.LBBackend {
			continue
		}

		route.LBFadeInDuration = 0
		route.LBFadeInExponent = 1
		endpointsCreated := make(map[string]time.Time)
		for _, f := range route.Filters {
			switch fi := f.Filter.(type) {
			case fadeIn:
				route.LBFadeInDuration = fi.duration
				route.LBFadeInExponent = fi.exponent
			case endpointCreated:
				endpointsCreated[fi.which] = fi.when
			}
		}

		for _, epi := range route.LBEndpoints {
			detected := r.GetMetrics(epi.Host).DetectedTime()
			if detected.IsZero() || endpointsCreated[epi.Host].After(detected) {
				r.SetDetectedTime(epi.Host, r.now())
			}
			r.lastSeen[epi.Host] = now

			r.mu.Lock()
			r.data[epi.Host].lastActive = now
			r.data[epi.Host].fadeInDuration = route.LBFadeInDuration
			r.mu.Unlock()
		}
	}

	for host, ts := range r.lastSeen {
		if ts.Add(lastSeenTimeout).Before(now) {
			delete(r.lastSeen, host)
			r.mu.Lock()
			delete(r.data, host)
			r.mu.Unlock()
		}
	}

	return routes
}

func NewEndpointRegistry(o RegistryOptions) *EndpointRegistry {
	return &EndpointRegistry{
		data:     map[string]*entry{},
		lastSeen: map[string]time.Time{},
		now:      time.Now,
	}
}

func (r *EndpointRegistry) GetMetrics(key string) Metrics {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	copy := &entry{}
	*copy = *e
	return copy
}

func (r *EndpointRegistry) SetDetectedTime(key string, detected time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	e.detected = detected
}

func (r *EndpointRegistry) IncInflightRequest(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	e.inflightRequests++
}

func (r *EndpointRegistry) DecInflightRequest(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.getOrInitEntryLocked(key)
	e.inflightRequests--
}

// getOrInitEntryLocked returns pointer to endpoint registry entry
// which contains the information about endpoint representing the
// following key. r.mu must be held while calling this function and
// using of the entry returned. In general, key represents the "host:port"
// string
func (r *EndpointRegistry) getOrInitEntryLocked(key string) *entry {
	e, ok := r.data[key]
	if !ok {
		e = &entry{}
		r.data[key] = e
	}
	return e
}
