/*
Package source implements a custom predicate to match routes
based on the source IP of a request.

It is similar in function and usage to the header predicate but
has explicit support for IP adresses and netmasks to conveniently
create routes based on a whole network of adresses, like a company
network or something similar.

It is important to note, that this predicate should not be used as
the only gatekeeper for secure endpoints. Always use proper authorization
and authentication for access control!

To enable usage of this predicate behind loadbalancers or proxies, the
X-Forwared-For header is used to determine the source of a request if it
is available. If the X-Forwarded-For header is not present or does not contain
a valid source address, the source IP of the incoming request is used for
matching.

The source predicate supports one or more IP addresses with or without a netmask.

Examples:

    // only match requests from 1.2.3.4
    example1: Source("1.2.3.4") -> "http://example.org";

    // only match requests from 1.2.3.0 - 1.2.3.255
    example2: Source("1.2.3.0/24") -> "http://example.org";

    // only match requests from 1.2.3.4 and the 2.2.2.0/24 network
    example3: Source("1.2.3.4", "2.2.2.0/24") -> "http://example.org";
*/
package source

import (
	"errors"
	snet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/routing"
	"net"
	"net/http"
	"strings"
)

var InvalidArgsError = errors.New("invalid arguments")

type spec struct{}

type predicate struct {
	acceptedSourceNets []net.IPNet
}

func New() routing.PredicateSpec { return &spec{} }

func (s *spec) Name() string {
	return "Source"
}

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) == 0 {
		return nil, InvalidArgsError
	}

	p := &predicate{}

	for i := range args {
		if s, ok := args[i].(string); ok {
			var netmask = s
			if !strings.Contains(s, "/") {
				netmask = s + "/32"
			}
			_, net, err := net.ParseCIDR(netmask)

			if err != nil {
				return nil, InvalidArgsError
			}

			p.acceptedSourceNets = append(p.acceptedSourceNets, *net)
		} else {
			return nil, InvalidArgsError
		}
	}

	return p, nil
}

func (p *predicate) Match(r *http.Request) bool {
	src := snet.RemoteHost(r)
	for _, acceptedNet := range p.acceptedSourceNets {
		if acceptedNet.Contains(src) {
			return true
		}
	}
	return false
}
