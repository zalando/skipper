package cors

import (
	"github.com/zalando/skipper/filters"
)

const (
	name              = "corsOrigin"
	allowOriginHeader = "Access-Control-Allow-Origin"
)

type basicSpec struct {
}

type filter struct {
	allowedOrigins []string
}

func NewOrigin() filters.Spec {
	return &basicSpec{}
}

// We check for the origin header if there are allowed origins
// otherwise we just set '*' as the value
func (a filter) Response(ctx filters.FilterContext) {
	if len(a.allowedOrigins) == 0 {
		ctx.Response().Header.Add(allowOriginHeader, "*")
		return
	}

	origin := ctx.Request().Header.Get("Origin")
	if origin == "" {
		return
	}
	for _, o := range a.allowedOrigins {
		if o == origin {
			ctx.Response().Header.Add(allowOriginHeader, o)
			return
		}
	}
}

// We do not touch request at all
func (a filter) Request(filters.FilterContext) {}

// Creates out the cors filter
func (spec basicSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	f := &filter{}
	for _, a := range args {
		if s, ok := a.(string); ok {
			f.allowedOrigins = append(f.allowedOrigins, s)
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
	}
	return f, nil
}

func (spec basicSpec) Name() string { return name }
