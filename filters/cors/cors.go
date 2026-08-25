package cors

import (
	"github.com/zalando/skipper/filters"
)

const (
	allowOriginHeader = "Access-Control-Allow-Origin"
)

type basicSpec struct {
}

type filter struct {
	allowedOrigins []string
}

// NewOrigin creates a CORS origin handler
// that can check for allowed origin or set an all allowed header
func NewOrigin() filters.Spec {
	return &basicSpec{}
}

// Response checks for the origin header if there are allowed origins
// otherwise it just sets '*' as the value
func (a filter) Response(ctx filters.FilterContext) {
	if len(a.allowedOrigins) == 0 {
		ctx.Response().Header.Set(allowOriginHeader, "*")
		return
	}

	origin := ctx.Request().Header.Get("Origin")
	if origin == "" {
		return
	}
	for _, o := range a.allowedOrigins {
		if o == origin {
			ctx.Response().Header.Set(allowOriginHeader, o)
			return
		}
	}
}

// Request is a noop
func (a filter) Request(filters.FilterContext) {}

// CreateFilter takes an optional string array.
// If any argument is not a string, it will return an error
func (spec basicSpec) CreateFilter(args []any) (filters.Filter, error) {
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

func (spec basicSpec) Name() string { return filters.CorsOriginName }
