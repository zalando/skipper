package settings

import (
	"github.com/mailgun/route"
	"net/http"
	"skipper/skipper"
)

type backend struct {
	scheme  string
	host    string
	isShunt bool
}

type filter struct {
	id string
}

type routedef struct {
	route   string
	backend *backend
	filters []skipper.Filter
}

type settings struct {
	routes route.Router
}

func (b *backend) Scheme() string {
	return b.scheme
}

func (b *backend) Host() string {
	return b.host
}

func (b *backend) IsShunt() bool {
	return b.isShunt
}

func (r *routedef) Backend() skipper.Backend {
	return r.backend
}

func (r *routedef) Filters() []skipper.Filter {
	return r.filters
}

func (s *settings) Route(r *http.Request) (skipper.Route, error) {
	rt, err := s.routes.Route(r)
	if rt == nil || err != nil {
		return nil, err
	}

	return rt.(skipper.Route), nil
}
