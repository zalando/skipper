package middleware

import "skipper/skipper"

import (
	// import middleware package here:

	"skipper/middleware/humanstxt"
	"skipper/middleware/pathrewrite"
	"skipper/middleware/requestheader"
	"skipper/middleware/responseheader"
	"skipper/middleware/xalando"
)

// takes a registry object and registers the middleware in the package
func Register(registry skipper.MiddlewareRegistry) {
	registry.Add(

		// add middleware to be used here:

		requestheader.Make(),
		responseheader.Make(),
		xalando.Make(),
		pathrewrite.Make(),
		humanstxt.Make(),
	)
}

// creates the default implementation of a skipper.MiddlewareRegistry object and registers the middleware in the
// package
func RegisterDefault() skipper.MiddlewareRegistry {
	r := makeRegistry()
	Register(r)
	return r
}
