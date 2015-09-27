package settings

import (
	"errors"
	"fmt"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/requestmatch"
	"log"
	"net/http"
	"net/url"
)

type Backend struct {
	Scheme  string
	Host    string
	IsShunt bool
}

type Route struct {
	Backend *Backend
	Filters []filters.Filter
}

type routedef struct {
	eskipRoute *eskip.Route
	value      *Route
}

type Settings struct {
	matcher *requestmatch.Matcher
}

func makeBackend(r *eskip.Route) (*Backend, error) {
	if r.Shunt {
		return &Backend{IsShunt: true}, nil
	}

	bu, err := url.ParseRequestURI(r.Backend)
	if err != nil {
		return nil, err
	}

	return &Backend{Scheme: bu.Scheme, Host: bu.Host}, nil
}

func makeFilter(ref *eskip.Filter, fr filters.Registry) (filters.Filter, error) {
	spec, ok := fr[ref.Name]
	if !ok {
		return nil, errors.New(fmt.Sprintf("filter not found: '%s'", ref.Name))
	}

	return spec.CreateFilter(ref.Args)
}

func makeFilters(r *eskip.Route, fr filters.Registry) ([]filters.Filter, error) {
	fs := make([]filters.Filter, len(r.Filters))
	for i, fspec := range r.Filters {
		f, err := makeFilter(fspec, fr)
		if err != nil {
			return nil, err
		}

		fs[i] = f
	}

	return fs, nil
}

func makeRouteDefinition(r *eskip.Route, fr filters.Registry) (*routedef, error) {
	b, err := makeBackend(r)
	if err != nil {
		return nil, err
	}

	fs, err := makeFilters(r, fr)
	if err != nil {
		return nil, err
	}

	rt := &Route{b, fs}
	return &routedef{r, rt}, nil
}

func makeMatcher(routes []*eskip.Route, fr filters.Registry, ignoreTrailingSlash bool) *requestmatch.Matcher {
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

func processRaw(rd string, fr filters.Registry, ignoreTrailingSlash bool) (*Settings, error) {
	d, err := eskip.Parse(rd)
	if err != nil {
		return nil, err
	}

	matcher := makeMatcher(d, fr, ignoreTrailingSlash)
	s := &Settings{matcher}
	return s, nil
}

func (rd *routedef) Id() string                         { return rd.eskipRoute.Id }
func (rd *routedef) Path() string                       { return rd.eskipRoute.Path }
func (rd *routedef) Method() string                     { return rd.eskipRoute.Method }
func (rd *routedef) HostRegexps() []string              { return rd.eskipRoute.HostRegexps }
func (rd *routedef) PathRegexps() []string              { return rd.eskipRoute.PathRegexps }
func (rd *routedef) Headers() map[string]string         { return rd.eskipRoute.Headers }
func (rd *routedef) HeaderRegexps() map[string][]string { return rd.eskipRoute.HeaderRegexps }
func (rd *routedef) Value() interface{}                 { return rd.value }

func (s *Settings) Route(r *http.Request) (*Route, error) {
	rt, _ := s.matcher.Match(r)
	if rt == nil {
		return nil, errors.New("route not found")
	}

	return rt.(*Route), nil
}
