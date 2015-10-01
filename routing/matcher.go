package routing

import (
	"fmt"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/requestmatch"
	"log"
	"net/url"
)

type routeDef struct {
	eskipRoute *eskip.Route
	value      *Route
}

func (rd *routeDef) Id() string                         { return rd.eskipRoute.Id }
func (rd *routeDef) Path() string                       { return rd.eskipRoute.Path }
func (rd *routeDef) Method() string                     { return rd.eskipRoute.Method }
func (rd *routeDef) HostRegexps() []string              { return rd.eskipRoute.HostRegexps }
func (rd *routeDef) PathRegexps() []string              { return rd.eskipRoute.PathRegexps }
func (rd *routeDef) Headers() map[string]string         { return rd.eskipRoute.Headers }
func (rd *routeDef) HeaderRegexps() map[string][]string { return rd.eskipRoute.HeaderRegexps }
func (rd *routeDef) Value() interface{}                 { return rd.value }

func createBackend(def *eskip.Route) (*Backend, error) {
	if def.Shunt {
		return &Backend{Shunt: true}, nil
	}

	bu, err := url.ParseRequestURI(def.Backend)
	if err != nil {
		return nil, err
	}

	bu = &url.URL{Scheme: bu.Scheme, Host: bu.Host}
	return &Backend{bu.Scheme, bu.Host, false}, nil
}

func (r *Routing) createFilter(def *eskip.Filter) (filters.Filter, error) {
	spec, ok := r.filterRegistry[def.Name]
	if !ok {
		return nil, fmt.Errorf("filter not found: '%s'", def.Name)
	}

	return spec.CreateFilter(def.Args)
}

func (r *Routing) createFilters(def *eskip.Route) ([]filters.Filter, error) {
	fs := make([]filters.Filter, len(def.Filters))
	for i, fdef := range def.Filters {
		f, err := r.createFilter(fdef)
		if err != nil {
			return nil, err
		}

		fs[i] = f
	}

	return fs, nil
}

func (r *Routing) convertDef(def *eskip.Route) (*routeDef, error) {
	b, err := createBackend(def)
	if err != nil {
		return nil, err
	}

	fs, err := r.createFilters(def)
	if err != nil {
		return nil, err
	}

	rt := &Route{b, fs}
	return &routeDef{def, rt}, nil
}

func (r *Routing) convertDefs(eskipDefs []*eskip.Route) []requestmatch.Definition {
	matcherDefs := []requestmatch.Definition{}
	for _, d := range eskipDefs {
		rd, err := r.convertDef(d)
		if err == nil {
			matcherDefs = append(matcherDefs, rd)
		} else {
			// idividual definition errors are accepted here
			log.Println(err)
		}
	}

	return matcherDefs
}

func (r *Routing) createMatcher(defs []requestmatch.Definition) *requestmatch.Matcher {
	m, errs := requestmatch.Make(defs, r.ignoreTrailingSlash)
	for _, err := range errs {
		// individual matcher entry errors are accepted here
		log.Println(err)
	}

	return m
}

func (r *Routing) processData(data string) (*requestmatch.Matcher, error) {
	eskipDefs, err := eskip.Parse(data)
	if err != nil {
		return nil, err
	}

	matcherDefs := r.convertDefs(eskipDefs)
	return r.createMatcher(matcherDefs), nil
}
