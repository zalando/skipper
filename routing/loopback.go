package routing

import (
	"context"
	"net/http"
)

type (
	routeChain struct {
		head *Route
		tail *routeChain
	}

	contextKey struct {
		name string
	}
)

var routeChainContextKey = &contextKey{"route-chain-context-key"}

// WithRoute returns a shallow copy of request with route stored in its context.
func WithRoute(request *http.Request, route *Route) *http.Request {
	parentCtx := request.Context()
	chain, _ := parentCtx.Value(routeChainContextKey).(*routeChain)
	chainCtx := context.WithValue(parentCtx, routeChainContextKey, &routeChain{route, chain})

	return request.WithContext(chainCtx)
}

func inLoop(l *leafMatcher, request *http.Request) bool {
	chain, _ := request.Context().Value(routeChainContextKey).(*routeChain)
	for chain != nil {
		if l.route == chain.head {
			return true
		}
		chain = chain.tail
	}
	return false
}
