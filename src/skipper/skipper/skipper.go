package skipper

import "net/http"

type RawData interface {

	// todo: consider what config format failures should be considered invalid
	// in json:
	// {
	//     "backends": {"pdp": "https://www.zalando.de/pdp.html"},
	//     "frontends": [{
	//         "route": "PathRegexp(`.*\\.html`)",
	//         "backendId": "pdp",
	//         "filters": [
	//             {"id": "pdp-custom-headers", "priority": 2},
	//             {"id": "x-session-id", "priority": 0}
	//         ]
	//     }],
	//     "filter-specs": {
	//         "pdp-custom-headers": {
	//             "middleware-name": "custom-headers",
	//             "config": {"free-data": 3.14}
	//         },
	//         "x-session-id": {
	//             "middleware-name": "x-session-id",
	//             "config": {"generator": "v4"}
	//         }
	//     }
	// }
	GetTestData() map[string]interface{}
}

type DataClient interface {
	Get() <-chan RawData
}

type Backend interface {
	Url() string
}

type MiddlewareConfig map[string]interface{}

type Filter interface {
	http.Handler
	Id() string
	Priority() int
}

type Route interface {
	Backend() Backend
	Filters() []Filter
}

type Settings interface {
	Route(*http.Request) (Route, error)
	Address() string
}

type SettingsSource interface {
	Get() <-chan Settings
}

type Middleware interface {
	Name() string
	MakeFilter(id string, priority int, s MiddlewareConfig) Filter
}

type MiddlewareRegistry interface {
	Add(...Middleware)
	Get(string) Middleware
	Remove(string)
}
