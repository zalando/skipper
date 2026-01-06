/*
Package source implements a custom predicate to match routes
based on the source IP of a request.

It is similar in function and usage to the header predicate but
has explicit support for IP addresses and netmasks to conveniently
create routes based on a whole network of addresses, like a company
network or something similar.

It is important to note, that this predicate should not be used as
the only gatekeeper for secure endpoints. Always use proper authorization
and authentication for access control!

To enable usage of this predicate behind load balancers or proxies, the
X-Forwarded-For header is used to determine the source of a request if it
is available. If the X-Forwarded-For header is not present or does not contain
a valid source address, the source IP of the incoming request is used for
matching.

The source predicate supports one or more IP addresses with or without a netmask.

There are two flavors of this predicate Source() and SourceFromLast().
The difference is that Source() finds the remote host as first entry from
the X-Forwarded-For header and SourceFromLast() as last entry.

Examples:

	// only match requests from 1.2.3.4
	example1: Source("1.2.3.4") -> "http://example.org";

	// only match requests from 1.2.3.0 - 1.2.3.255
	example2: Source("1.2.3.0/24") -> "http://example.org";

	// only match requests from 1.2.3.4 and the 2.2.2.0/24 network
	example3: Source("1.2.3.4", "2.2.2.0/24") -> "http://example.org";

	// same as example3, only match requests from 1.2.3.4 and the 2.2.2.0/24 network
	example4: SourceFromLast("1.2.3.4", "2.2.2.0/24") -> "http://example.org";
*/
package source

import (
	"errors"
	"net"
	"net/http"
	"net/netip"

	snet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
	"go4.org/netipx"
)

const (
	// Deprecated, use predicates.SourceName instead
	Name = predicates.SourceName
	// Deprecated, use predicates.SourceFromLastName instead
	NameLast = predicates.SourceFromLastName
	// Deprecated, use predicates.ClientIPName instead
	NameClientIP = predicates.ClientIPName
)

var errInvalidArgs = errors.New("invalid arguments")

type sourcePred int

const (
	source sourcePred = iota
	sourceFromLast
	clientIP
)

type spec struct {
	typ sourcePred
}

type predicate struct {
	typ  sourcePred
	nets *netipx.IPSet
}

func New() routing.PredicateSpec         { return &spec{typ: source} }
func NewFromLast() routing.PredicateSpec { return &spec{typ: sourceFromLast} }
func NewClientIP() routing.PredicateSpec { return &spec{typ: clientIP} }

func (s *spec) Name() string {
	switch s.typ {
	case sourceFromLast:
		return predicates.SourceFromLastName
	case clientIP:
		return predicates.ClientIPName
	default:
		return predicates.SourceName
	}
}

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) == 0 {
		return nil, errInvalidArgs
	}

	var cidrs []string
	for i := range args {
		if s, ok := args[i].(string); ok {
			cidrs = append(cidrs, s)
		} else {
			return nil, errInvalidArgs
		}
	}

	nets, err := snet.ParseIPCIDRs(cidrs)
	if err != nil {
		return nil, err
	}

	return &predicate{s.typ, nets}, nil
}

func (p *predicate) Match(r *http.Request) bool {
	var src netip.Addr
	switch p.typ {
	case sourceFromLast:
		src = snet.RemoteAddrFromLast(r)
	case clientIP:
		h, _, _ := net.SplitHostPort(r.RemoteAddr)
		src, _ = netip.ParseAddr(h)
	default:
		src = snet.RemoteAddr(r)
	}
	return p.nets.Contains(src)
}
