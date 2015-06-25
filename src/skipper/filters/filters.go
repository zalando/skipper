package filters

import "skipper/skipper"

import (
	// import filter packages here:

	"skipper/filters/healthcheck"
	"skipper/filters/humanstxt"
	"skipper/filters/pathrewrite"
	"skipper/filters/requestheader"
	"skipper/filters/responseheader"
	"skipper/filters/xalando"
)

// takes a registry object and registers the filter spec in the package
func Register(registry skipper.FilterRegistry) {
	registry.Add(

		// add filter specs to be used here:

		requestheader.Make(),
		responseheader.Make(),
		xalando.Make(),
		pathrewrite.Make(),
		healthcheck.Make(),
		humanstxt.Make(),
	)
}

// creates the default implementation of a skipper.FilterRegistry object and registers the filter specs in the
// package
func RegisterDefault() skipper.FilterRegistry {
	r := makeRegistry()
	Register(r)
	return r
}
