package settings

import (
	"errors"
	"fmt"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/routematcher"
	"github.com/zalando/skipper/skipper"
	"log"
	"net/url"
)

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

func makeFilter(id string, spec *eskip.Filter, fr skipper.FilterRegistry) (skipper.Filter, error) {
	mw := fr.Get(spec.Name)
	if mw == nil {
		return nil, errors.New(fmt.Sprintf("filter not found: '%s' '%s'", id, spec.Name))
	}

	return mw.MakeFilter(id, skipper.FilterConfig(spec.Args))
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

func makeMatcher(routes []*eskip.Route, fr skipper.FilterRegistry, ignoreTrailingSlash bool) *routematcher.Matcher {
	routeDefinitions := make([]routematcher.RouteDefinition, len(routes))
	for i, r := range routes {
		rd, err := makeRouteDefinition(r, fr)
		if err != nil {
			log.Println(err)
		}

		routeDefinitions[i] = rd
	}

	router, errs := routematcher.Make(routeDefinitions, ignoreTrailingSlash)
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
