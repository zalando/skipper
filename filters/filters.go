package filters

import (
	// import filter packages here:

	"github.com/zalando/skipper/filters/healthcheck"
	"github.com/zalando/skipper/filters/humanstxt"
	"github.com/zalando/skipper/filters/pathrewrite"
	"github.com/zalando/skipper/filters/requestheader"
	"github.com/zalando/skipper/filters/responseheader"
	"github.com/zalando/skipper/filters/static"
	"github.com/zalando/skipper/filters/stripquery"
	"github.com/zalando/skipper/skipper"
)

// takes a registry object and registers the filter spec in the package
func Register(registry skipper.FilterRegistry) {
	registry.Add(

		// add filter specs to be used here:

		requestheader.Make(),
		responseheader.Make(),
		pathrewrite.Make(),
		healthcheck.Make(),
		humanstxt.Make(),
		static.Make(),
		stripquery.Make(),
	)
}

// creates the default implementation of a skipper.FilterRegistry object and registers the filter specs in the
// package
func RegisterDefault() skipper.FilterRegistry {
	r := makeRegistry()
	Register(r)
	return r
}
