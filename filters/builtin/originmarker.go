package builtin

import (
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.OriginMarkerName instead
	OriginMarkerName = filters.OriginMarkerName
)

type originMarkerSpec struct{}

// OriginMarker carries information about the origin of a route
type OriginMarker struct {
	// the type of origin, e.g. ingress
	Origin string `json:"origin"`
	// the unique ID (within the origin) of the source object (e.g. ingress) that created the route
	Id string `json:"id"`
	// when the source object was created
	Created time.Time `json:"created"`
}

// NewOriginMarkerSpec creates a filter specification whose instances
// mark the origin of an eskip.Route
func NewOriginMarkerSpec() filters.Spec {
	return &originMarkerSpec{}
}

func NewOriginMarker(origin string, id string, created time.Time) *eskip.Filter {
	return &eskip.Filter{
		Name: filters.OriginMarkerName,
		Args: []any{origin, id, created},
	}
}

func (s *originMarkerSpec) Name() string { return filters.OriginMarkerName }

func (s *originMarkerSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	f := &OriginMarker{}

	if value, ok := args[0].(string); ok {
		f.Origin = value
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}

	if value, ok := args[1].(string); ok {
		f.Id = value
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}

	switch created := args[2].(type) {
	case time.Time:
		f.Created = created
	case string:
		if value, err := time.Parse(time.RFC3339, created); err == nil {
			f.Created = value
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	return f, nil
}

func (m OriginMarker) Request(filters.FilterContext) {}

func (m OriginMarker) Response(filters.FilterContext) {}
