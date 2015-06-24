package middleware

import "skipper/skipper"

// default implementation of skipper.MiddlewareRegistry
type registry struct {
	mw map[string]skipper.Middleware
}

func makeRegistry() skipper.MiddlewareRegistry {
	return &registry{map[string]skipper.Middleware{}}
}

func (r *registry) Add(mw ...skipper.Middleware) {
	for _, mwi := range mw {
		r.mw[mwi.Name()] = mwi
	}
}

func (r *registry) Get(name string) skipper.Middleware {
	return r.mw[name]
}

func (r *registry) Remove(name string) {
	delete(r.mw, name)
}
