package kubernetes

import (
	"encoding/json"
	"errors"
)

var errInvalidSkipperBackendType = errors.New("invalid skipper backend type")

type routeGroupList struct {
	Items []*routeGroupItem `json:"items"`
}

type routeGroupItem struct {
	Metadata *metadata       `json:"metadata"`
	Spec     *routeGroupSpec `json:"spec"`
}

type routeGroupSpec struct {
	Hosts           []string          `json:"hosts,omitempty"`
	DefaultBackends []*skipperBackend `json:"backends"`
	Paths           []*pathGroup      `json:"paths"`
}

type pathGroup struct {
	Path   string      `json:"path"`
	Method string      `json:"method,omitempty"`
	Config configGroup `json:"config,omitempty"`
}

type configGroup struct {
	Filters    []*filter       `json:"filters,omitempty"`
	Predicates []*predicate    `json:"predicates,omitempty"`
	Backend    *skipperBackend `json:"backend,omitempty"`
}

type filter string
type predicate string

// can be:
// - *backend defined in definitions.go
// - SpecialBackend string   // <shunt>, ..
type skipperBackend struct {
	value interface{}
}

func (sb skipperBackend) special() (string, bool) {
	s, ok := sb.value.(string)
	return s, ok
}

func (sb skipperBackend) backend() (backend, bool) {
	b, ok := sb.value.(backend)
	return b, ok
}

func (sb *skipperBackend) UnmarshalJSON(value []byte) error {
	if value[0] == '"' { // TODO(sszuecs): correct? or '<'
		var s string
		if err := json.Unmarshal(value, &s); err != nil {
			return err
		}

		sb.value = s
		return nil
	}

	var b backend
	if err := json.Unmarshal(value, &b); err != nil {
		return err
	}

	sb.value = b
	return nil
}

func (sb skipperBackend) MarshalJSON() ([]byte, error) {
	switch sb.value.(type) {
	case string, backend:
		return json.Marshal(sb.value)
	default:
		return nil, errInvalidSkipperBackendType
	}
}

func (sb skipperBackend) String() string {
	switch v := sb.value.(type) {
	case string:
		return v
	case backend:
		return v.String()
	default:
		return ""
	}
}
