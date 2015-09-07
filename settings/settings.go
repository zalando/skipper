package settings

import (
	"errors"
	"fmt"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/requestmatch"
	"github.com/zalando/skipper/skipper"
	"log"
	"net/http"
	"net/url"
)

type backend struct {
	scheme  string
	host    string
	isShunt bool
}

type filter struct {
	id string
}

type route struct {
	backend skipper.Backend
	filters []skipper.Filter
}

type routedef struct {
	eskipRoute *eskip.Route
	value      *route
}

type settings struct {
	matcher *requestmatch.Matcher
}

func makeBackend(r *eskip.Route) (*backend, error) {
	if r.Shunt {
		return &backend{isShunt: true}, nil
	}

	bu, err := url.ParseRequestURI(r.Backend)
	if err != nil {
		return nil, err
	}

	return &backend{scheme: bu.Scheme, host: bu.Host}, nil
}

func makeFilterId(routeId, filterName string, index int) string {
	return fmt.Sprintf("%s-%s-%d", routeId, filterName, index)
}

func makeFilter(id string, ref *eskip.Filter, fr skipper.FilterRegistry) (skipper.Filter, error) {
	spec := fr.Get(ref.Name)
	if spec == nil {
		return nil, errors.New(fmt.Sprintf("filter not found: '%s' '%s'", id, ref.Name))
	}

	return spec.MakeFilter(id, skipper.FilterConfig(ref.Args))
}

func makeFilters(r *eskip.Route, fr skipper.FilterRegistry) ([]skipper.Filter, error) {
	fs := make([]skipper.Filter, len(r.Filters))
	for i, fspec := range r.Filters {
		f, err := makeFilter(makeFilterId(r.Id, fspec.Name, i), fspec, fr)
		if err != nil {
			return nil, err
		}

		fs[i] = f
	}

	return fs, nil
}

func makeRouteDefinition(r *eskip.Route, fr skipper.FilterRegistry) (*routedef, error) {
	b, err := makeBackend(r)
	if err != nil {
		return nil, err
	}

	fs, err := makeFilters(r, fr)
	if err != nil {
		return nil, err
	}

	rt := &route{b, fs}
	return &routedef{r, rt}, nil
}

func makeMatcher(routes []*eskip.Route, fr skipper.FilterRegistry, ignoreTrailingSlash bool) *requestmatch.Matcher {
	routeDefinitions := make([]requestmatch.Definition, len(routes))
	for i, r := range routes {
		rd, err := makeRouteDefinition(r, fr)
		if err != nil {
			log.Println(err)
		}

		routeDefinitions[i] = rd
	}

	router, errs := requestmatch.Make(routeDefinitions, ignoreTrailingSlash)
	for _, err := range errs {
		log.Println(err)
	}

	return router
}

func processRaw(rd skipper.RawData, fr skipper.FilterRegistry, ignoreTrailingSlash bool) (skipper.Settings, error) {
	d, err := eskip.Parse(rd.Get())
	if err != nil {
		return nil, err
	}

	matcher := makeMatcher(d, fr, ignoreTrailingSlash)
	s := &settings{matcher}
	return s, nil
}

func (b *backend) Scheme() string { return b.scheme }
func (b *backend) Host() string   { return b.host }
func (b *backend) IsShunt() bool  { return b.isShunt }

func (r *route) Backend() skipper.Backend  { return r.backend }
func (r *route) Filters() []skipper.Filter { return r.filters }

func (rd *routedef) Id() string                         { return rd.eskipRoute.Id }
func (rd *routedef) Path() string                       { return rd.eskipRoute.Path }
func (rd *routedef) Method() string                     { return rd.eskipRoute.Method }
func (rd *routedef) HostRegexps() []string              { return rd.eskipRoute.HostRegexps }
func (rd *routedef) PathRegexps() []string              { return rd.eskipRoute.PathRegexps }
func (rd *routedef) Headers() map[string]string         { return rd.eskipRoute.Headers }
func (rd *routedef) HeaderRegexps() map[string][]string { return rd.eskipRoute.HeaderRegexps }
func (rd *routedef) Value() interface{}                 { return rd.value }

func (s *settings) Route(r *http.Request) (skipper.Route, error) {
    println("routing")
	rt, _ := s.matcher.Match(r)
	if rt == nil {
		return nil, errors.New("route not found")
	}

	return rt.(skipper.Route), nil
}
