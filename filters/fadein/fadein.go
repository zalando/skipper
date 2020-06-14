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
	DurationName        = "fadeInDuration"
	EaseName            = "fadeInEase"
	EndpointCreatedName = "endpointCreated"
)

type (
	duration time.Duration
	ease     float64

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
		detected map[string]detectedFadeIn
	}
)

func NewDuration() filters.Spec {
	return duration(0)
}

func (duration) Name() string { return DurationName }

func (duration) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	switch v := args[0].(type) {
	case int:
		return duration(v * int(time.Millisecond)), nil
	case float64:
		return duration(int(v) * int(time.Millisecond)), nil
	case string:
		d, err := time.ParseDuration(v)
		return duration(d), err
	case time.Duration:
		return duration(v), nil
	default:
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (duration) Request(filters.FilterContext)  {}
func (duration) Response(filters.FilterContext) {}

func NewEase() filters.Spec {
	return ease(0)
}

func (ease) Name() string { return EaseName }

func (ease) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	switch v := args[0].(type) {
	case int:
		return ease(v), nil
	case float64:
		return ease(v), nil
	default:
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (ease) Request(filters.FilterContext)  {}
func (ease) Response(filters.FilterContext) {}

func NewEndpointCreated() filters.Spec {
	var ec endpointCreated
	return ec
}

func (endpointCreated) Name() string { return EndpointCreatedName }

func normalizeSchemeHost(s, h string) (string, string, error) {
	// endpoint address cannot contain path, the rest is not case sensitive
	s, h = strings.ToLower(s), strings.ToLower(h)

	h, p, err := net.SplitHostPort(h)
	if err != nil {
		return "", "", err
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

	// mitigate potential flakyness caused by clock skew. When the created time is in the future based on
	// the local clock, we ignore it.
	now := time.Now()
	if ec.when.After(now) {
		log.Errorf("Endpoint created time in the future: %v. Potential clock skew.", ec.when)
		ec.when = time.Time{}
	}

	return ec, nil
}

func (endpointCreated) Request(filters.FilterContext)  {}
func (endpointCreated) Response(filters.FilterContext) {}

func NewPostProcessor() routing.PostProcessor {
	return &postProcessor{
		detected: make(map[string]detectedFadeIn),
	}
}

func (p *postProcessor) Do(r []*routing.Route) []*routing.Route {
	const configErrFmt = "Error while processing endpoint fade-in settings: %s, %s, %v."
	now := time.Now()

	for _, ri := range r {
		if ri.Route.BackendType != eskip.LBBackend {
			continue
		}

		ri.LBFadeInDuration = 0
		ri.LBFadeInEase = 1
		endpointsCreated := make(map[string]time.Time)
		for _, f := range ri.Filters {
			if d, ok := f.Filter.(duration); ok {
				ri.LBFadeInDuration = time.Duration(d)
			}

			if e, ok := f.Filter.(ease); ok {
				ri.LBFadeInEase = float64(e)
			}

			if ec, ok := f.Filter.(endpointCreated); ok {
				endpointsCreated[ec.which] = ec.when
			}
		}

		if ri.LBFadeInDuration <= 0 {
			continue
		}

		for i := range ri.LBEndpoints {
			ep := &ri.LBEndpoints[i]

			s, h, err := normalizeSchemeHost(ep.Scheme, ep.Host)
			if err != nil {
				log.Errorf(configErrFmt, ep.Scheme, ep.Host, err)
				continue
			}

			key := endpointKey(s, h)
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
