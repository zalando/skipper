package definitions

import (
	"fmt"

	"errors"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
)

// adding Kubernetes specific backend types here. To be discussed.
// The main reason is to differentiate between service and external, in a way
// where we can also use the current global option to decide whether the service
// should then be converted to LB. Or shall we expect the route group already
// contain the pod endpoints, and ignore the global option for skipper?
// --> As CRD we have to lookup endpoints ourselves, maybe via kube.go
const (
	ServiceBackend = eskip.LBBackend + 1 + iota
)

var (
	errRouteGroupWithoutBackend = errors.New("route group without backend")
	errRouteGroupWithoutName    = errors.New("route group without name")
	errRouteGroupWithoutSpec    = errors.New("route group without spec")
	errInvalidRouteSpec         = errors.New("invalid route spec")
	errInvalidPredicate         = errors.New("invalid predicate")
	errInvalidFilter            = errors.New("invalid filter")
	errInvalidMethod            = errors.New("invalid method")
	errBothPathAndPathSubtree   = errors.New("path and path subtree in the same route")
	errMissingBackendReference  = errors.New("missing backend reference")
	errUnnamedBackend           = errors.New("unnamed backend")
	errUnnamedBackendReference  = errors.New("unnamed backend reference")
)

type RouteGroupList struct {
	Items []*RouteGroupItem `json:"items"`
}

type RouteGroupItem struct {
	Metadata *Metadata       `json:"metadata"`
	Spec     *RouteGroupSpec `json:"spec"`
}

type RouteGroupSpec struct {
	// Hosts specifies the host headers, that will be matched for
	// all routes created by this route group. No hosts mean
	// catchall.
	Hosts []string `json:"hosts,omitempty"`

	// Backends specify the list of backends that can be
	// referenced from routes or DefaultBackends.
	Backends []*SkipperBackend `json:"backends"`

	// DefaultBackends should be in most cases only one default
	// backend which is applied to all routes, if no override was
	// added to a route. A special case is Traffic Switching which
	// will have more than one default backend definition.
	DefaultBackends BackendReferences `json:"defaultBackends,omitempty"`

	// Routes specifies the list of route based on path, method
	// and predicates. It defaults to catchall, if there are no
	// routes.
	Routes []*RouteSpec `json:"routes,omitempty"`
}

// SkipperBackend is the type safe version of skipperBackendParser
type SkipperBackend struct {
	// Name is the backendName that can be referenced as backendReference
	Name string

	// Type is the parsed backend type
	Type eskip.BackendType

	// Address is required for Type network. Address follows the
	// URL spec without path, query and fragment. For example
	// https://user:password@example.org
	Address string

	// ServiceName is required for Type service
	ServiceName string

	// ServicePort is required for Type service
	ServicePort int

	// Algorithm is required for Type lb
	Algorithm loadbalancer.Algorithm

	// Endpoints is required for Type lb
	Endpoints []string

	parseError error
}

// skipperBackendParser is an intermediate type required for parsing
// skipperBackend and adding type safety for Algorithm and Type with
// skipperBackend type.
type skipperBackendParser struct {
	// Name is the backendName that can be referenced as backendReference
	Name string `json:"name"`

	// Type is one of "service|shunt|loopback|dynamic|lb|network"
	Type string `json:"type"`

	// Address is required for Type network
	Address string `json:"address"`

	// Algorithm is required for Type lb
	Algorithm string `json:"algorithm"`

	// Endpoints is required for Type lb
	Endpoints []string `json:"endpoints"`

	// ServiceName is required for Type service
	ServiceName string `json:"serviceName"`

	// ServicePort is required for Type service
	ServicePort int `json:"servicePort"`
}

type BackendReference struct {
	// BackendName references the skipperBackend by name
	BackendName string `json:"backendName"`

	// Weight defines the traffic weight, if there are 2 or more
	// default backends
	Weight int `json:"weight"`
}

type BackendReferences []*BackendReference

var _ WeightedBackend = &BackendReference{}

func (br *BackendReference) GetName() string    { return br.BackendName }
func (br *BackendReference) GetWeight() float64 { return float64(br.Weight) }

type RouteSpec struct {
	// Path specifies Path predicate, only one of Path or PathSubtree is allowed
	Path string `json:"path,omitempty"`

	// PathSubtree specifies PathSubtree predicate, only one of Path or PathSubtree is allowed
	PathSubtree string `json:"pathSubtree,omitempty"`

	// PathRegexp can be added additionally
	PathRegexp string `json:"pathRegexp,omitempty"`

	// Backends specifies the list of backendReference that should
	// be applied to override the defaultBackends
	Backends BackendReferences `json:"backends,omitempty"`

	// Filters specifies the list of filters applied to the RouteSpec
	Filters []string `json:"filters,omitempty"`

	// Predicates specifies the list of predicates applied to the RouteSpec
	Predicates []string `json:"predicates,omitempty"`

	// Methods defines valid HTTP methods for the specified RouteSpec
	Methods []string `json:"methods,omitempty"`
}

func backendsWithDuplicateName(name string) error {
	return fmt.Errorf("backends with duplicate name: %s", name)
}

func invalidBackend(name string, err error) error {
	return fmt.Errorf("invalid backend: %s, %w", name, err)
}

func invalidBackendReference(name string) error {
	return fmt.Errorf("invalid backend reference: %s", name)
}

func duplicateBackendReference(name string) error {
	return fmt.Errorf("duplicate backend reference: %s", name)
}

func invalidBackendWeight(name string, w int) error {
	return fmt.Errorf("invalid weight in backend: %s, %d", name, w)
}

func invalidRoute(index int, err error) error {
	return fmt.Errorf("invalid route at %d, %w", index, err)
}

func missingAddress(backendName string) error {
	return fmt.Errorf("address missing in backend: %s", backendName)
}

func missingServiceName(backendName string) error {
	return fmt.Errorf("service name missing in backend: %s", backendName)
}

func invalidServicePort(backendName string, p int) error {
	return fmt.Errorf("invalid service port in backend: %s, %d", backendName, p)
}

func missingEndpoints(backendName string) error {
	return fmt.Errorf("missing LB endpoints in backend: %s", backendName)
}

func routeGroupError(m *Metadata, err error) error {
	return fmt.Errorf("error in route group %s/%s: %w", namespaceString(m.Namespace), m.Name, err)
}
