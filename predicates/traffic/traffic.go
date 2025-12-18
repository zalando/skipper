/*
Package traffic implements a predicate to control the matching
probability for a given route by setting its weight.

The probability for matching a route is defined by the mandatory first
parameter, that must be a decimal number between 0.0 and 1.0 (both
inclusive).

The optional second argument is used to specify the cookie name for
the traffic group, in case you want to use stickiness. Stickiness
allows all subsequent requests from the same client to match the same
route. Stickiness of traffic is supported by the optional third
parameter, indicating whether the request being matched belongs to the
traffic group of the current route. If yes, the predicate matches
ignoring the chance argument.

You always have to specify one argument, if you do not need stickiness,
and three arguments, if your service requires stickiness.

Predicates cannot modify the response, so the responsibility of
setting the traffic group cookie remains to either a filter or the
backend service.

The below example, shows a possible eskip document used for green-blue
deployments of APIS, which usually don't require stickiness:

	// hit by 10% percent chance
	v2:
	    Traffic(.1) ->
	    "https://api-test-green";

	// hit by remaining chance
	v1:
	    "https://api-test-blue";

The below example, shows a possible eskip document with two,
independent traffic controlled route sets, which uses session stickiness:

	// hit by 5% percent chance
	cartTest:
	    Traffic(.05, "cart-test", "test") && Path("/cart") ->
	    responseCookie("cart-test", "test") ->
	    "https://cart-test";

	// hit by remaining chance
	cart:
	    Path("/cart") ->
	    responseCookie("cart-test", "default") ->
	    "https://cart";

	// hit by 15% percent chance
	catalogTestA:
	    Traffic(.15, "catalog-test", "A") ->
	    responseCookie("catalog-test", "A") ->
	    "https://catalog-test-a";

	// hit by 30% percent chance
	catalogTestB:
	    Traffic(.3, "catalog-test", "B") ->
	    responseCookie("catalog-test", "B") ->
	    "https://catalog-test-b";

	// hit by remaining chance
	catalog:
	    * ->
	    responseCookie("catalog-test", "default") ->
	    "https://catalog";
*/
package traffic

import (
	"math/rand/v2"
	"net/http"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	// Deprecated, use predicates.TrafficName instead
	PredicateName = predicates.TrafficName
)

type spec struct{}

type predicate struct {
	chance             float64
	trafficGroup       string
	trafficGroupCookie string
}

// New creates a new traffic control predicate specification.
func New() routing.PredicateSpec { return &spec{} }

func (s *spec) Name() string { return predicates.TrafficName }

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if !(len(args) == 1 || len(args) == 3) {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	p := &predicate{}

	if c, ok := args[0].(float64); ok && 0.0 <= c && c <= 1.0 {
		p.chance = c
	} else {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	if len(args) == 3 {
		if tgc, ok := args[1].(string); ok {
			p.trafficGroupCookie = tgc
		} else {
			return nil, predicates.ErrInvalidPredicateParameters
		}
		if tg, ok := args[2].(string); ok {
			p.trafficGroup = tg
		} else {
			return nil, predicates.ErrInvalidPredicateParameters
		}
	}

	return p, nil
}

func (p *predicate) takeChance() bool {
	return rand.Float64() < p.chance // #nosec
}

func (p *predicate) Match(r *http.Request) bool {
	if p.trafficGroup == "" {
		return p.takeChance()
	}

	if c, err := r.Cookie(p.trafficGroupCookie); err == nil {
		return c.Value == p.trafficGroup
	} else {
		return p.takeChance()
	}
}
