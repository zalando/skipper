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
}

func makeFilter(ref *eskip.Filter, fr filters.Registry) (filters.Filter, error) {
}

func makeFilters(r *eskip.Route, fr filters.Registry) ([]filters.Filter, error) {
}

func makeRouteDefinition(r *eskip.Route, fr filters.Registry) (*routedef, error) {
}

func processRaw(rd string, fr filters.Registry, ignoreTrailingSlash bool) (*Settings, error) {
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
