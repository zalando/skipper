package settings

import (
	"errors"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/routematcher"
	"github.com/zalando/skipper/skipper"
	"net/http"
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
	matcher *routematcher.Matcher
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
	rt, _ := s.matcher.Match(r)
	if rt == nil {
		return nil, errors.New("route not found")
	}

	return rt.(skipper.Route), nil
}
