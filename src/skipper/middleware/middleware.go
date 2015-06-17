package middleware

import "skipper/skipper"

import (
	// import middleware package here:
	"skipper/middleware/requestheader"
	"skipper/middleware/xalando"
)

func Register(registry skipper.MiddlewareRegistry) {
	registry.Add(

		// add middleware to be used here:
		requestheader.Make(),
		xalando.Make(),
	)
}

func RegisterToDefault() skipper.MiddlewareRegistry {
	r := makeRegistry()
	Register(r)
	return r
}
