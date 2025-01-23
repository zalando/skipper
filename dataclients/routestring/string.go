// Package routestring provides a DataClient implementation for
// setting route configuration in form of simple eskip string.
//
// Usage from the command line:
//
//	skipper -inline-routes '* -> inlineContent("Hello, world!") -> <shunt>'
package routestring

import (
	"fmt"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

type routes struct {
	parsed []*eskip.Route
}

// New creates a data client that parses a string of eskip routes and
// serves it for the routing package.
func New(r string) (routing.DataClient, error) {
	parsed, err := eskip.Parse(r)
	if err != nil {
		return nil, err
	}
	return &routes{parsed: parsed}, nil
}

// NewList creates a data client that parses a list of strings of eskip routes and
// serves it for the routing package.
func NewList(rs []string) (routing.DataClient, error) {
	var parsed []*eskip.Route
	for i, r := range rs {
		pr, err := eskip.Parse(r)
		if err != nil {
			return nil, fmt.Errorf("#%d: %w", i, err)
		}
		parsed = append(parsed, pr...)
	}
	return &routes{parsed: parsed}, nil
}

func (r *routes) LoadAll() ([]*eskip.Route, error) {
	return r.parsed, nil
}

func (*routes) LoadUpdate() ([]*eskip.Route, []string, error) {
	return nil, nil, nil
}
