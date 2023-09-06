package fadein

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

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

	detectedFadeIn struct {
		when       time.Time
		duration   time.Duration
		lastActive time.Time
	}

	postProcessor struct {
		// "http://10.2.1.53:1234": {t0 60s t0-10s}
		detected map[string]detectedFadeIn
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

func endpointKey(scheme, host string) string {
	return fmt.Sprintf("%s://%s", scheme, host)
}

func (endpointCreated) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	e, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	s, h, err := normalizeEndpoint(e)
	if err != nil {
		return nil, err
	}

	ec := endpointCreated{which: endpointKey(s, h)}
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

// NewPostProcessor creates post-processor for maintaining the detection time of LB endpoints with fade-in
// behavior.
func NewPostProcessor() routing.PostProcessor {
	return &postProcessor{
		detected: make(map[string]detectedFadeIn),
	}
}

func (p *postProcessor) Do(r []*routing.Route) []*routing.Route {
	now := time.Now()

	for _, ri := range r {
		if ri.Route.BackendType != eskip.LBBackend {
			continue
		}

		ri.LBFadeInDuration = 0
		ri.LBFadeInExponent = 1
		endpointsCreated := make(map[string]time.Time)
		for _, f := range ri.Filters {
			switch fi := f.Filter.(type) {
			case fadeIn:
				ri.LBFadeInDuration = fi.duration
				ri.LBFadeInExponent = fi.exponent
			case endpointCreated:
				endpointsCreated[fi.which] = fi.when
			}
		}

		if ri.LBFadeInDuration <= 0 {
			continue
		}

		for i := range ri.LBEndpoints {
			ep := &ri.LBEndpoints[i]

			key := endpointKey(ep.Scheme, ep.Host)
			detected := p.detected[key].when
			if detected.IsZero() || endpointsCreated[key].After(detected) {
				detected = now
			}

			ep.Detected = detected
			p.detected[key] = detectedFadeIn{
				when:       detected,
				duration:   ri.LBFadeInDuration,
				lastActive: now,
			}
		}
	}

	// cleanup old detected, considering last active
	for key, d := range p.detected {
		// this allows tolerating when a fade-in endpoint temporarily disappears:
		if d.lastActive.Add(d.duration).Before(now) {
			delete(p.detected, key)
		}
	}

	return r
}
