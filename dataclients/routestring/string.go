// Package routestring provides a DataClient implementation for
// setting route configuration in form of simple eskip string.
package routestring

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

type routes struct {
	parsed []*eskip.Route
}

// New creates a data client that parses a string of eskip routes and
// serves it for the routing package.
//
// Usage from the command line:
//
//     skipper -inline-routes '* -> inlineContent("Hello, world!") -> <shunt>'
//
func New(r string) (routing.DataClient, error) {
	parsed, err := eskip.Parse(r)
	if err != nil {
		return nil, err
	}

	return &routes{parsed: parsed}, nil
}

func (r *routes) LoadAll() ([]*eskip.Route, error) {
	return r.parsed, nil
}

func (_ *routes) LoadUpdate() ([]*eskip.Route, []string, error) {
	return nil, nil, nil
}
