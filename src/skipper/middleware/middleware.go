package middleware

import "skipper/skipper"

import (
	// import middleware package here:
	// "skipper/middleware/requestheader"
	// "skipper/middleware/responseheader"
	"skipper/middleware/xalando"
)

func Register(registry skipper.MiddlewareRegistry) {
	registry.Add(

		// add middleware to be used here:
		xalando.Make(),
	)
}

func RegisterDefault() skipper.MiddlewareRegistry {
	r := makeRegistry()
	Register(r)
	return r
}
