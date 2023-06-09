package definitions

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
	"gopkg.in/yaml.v2"
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

func uniqueStrings(s []string) []string {
	u := make([]string, 0, len(s))
	m := make(map[string]bool)
	for _, si := range s {
		if m[si] {
			continue
		}

		m[si] = true
		u = append(u, si)
	}

	return u
}

func backendTypeFromString(s string) (eskip.BackendType, error) {
	switch s {
	case "", "service":
		return ServiceBackend, nil
	default:
		return eskip.BackendTypeFromString(s)
	}
}

func (sb *SkipperBackend) validate() error {
	if sb.parseError != nil {
		return sb.parseError
	}

	if sb == nil || sb.Name == "" {
		return errUnnamedBackend
	}

	switch {
	case sb.Type == eskip.NetworkBackend && sb.Address == "":
		return missingAddress(sb.Name)
	case sb.Type == ServiceBackend && sb.ServiceName == "":
		return missingServiceName(sb.Name)
	case sb.Type == ServiceBackend &&
		(sb.ServicePort == 0 || sb.ServicePort != int(uint16(sb.ServicePort))):
		return invalidServicePort(sb.Name, sb.ServicePort)
	case sb.Type == eskip.LBBackend && len(sb.Endpoints) == 0:
		return missingEndpoints(sb.Name)
	}

	return nil
}

// UnmarshalJSON creates a new skipperBackend, safe to be called on nil pointer
func (sb *SkipperBackend) UnmarshalJSON(value []byte) error {
	if sb == nil {
		return nil
	}

	var p skipperBackendParser
	if err := json.Unmarshal(value, &p); err != nil {
		return err
	}

	var perr error
	bt, err := backendTypeFromString(p.Type)
	if err != nil {
		// we cannot return an error here, because then the parsing of
		// all route groups would fail. We'll report the error in the
		// validation phase, only for the containing route group
		perr = err
	}

	a, err := loadbalancer.AlgorithmFromString(p.Algorithm)
	if err != nil {
		// we cannot return an error here, because then the parsing of
		// all route groups would fail. We'll report the error in the
		// validation phase, only for the containing route group
		perr = err
	}

	if a == loadbalancer.None {
		a = loadbalancer.RoundRobin
	}

	var b SkipperBackend
	b.Name = p.Name
	b.Type = bt
	b.Address = p.Address
	b.ServiceName = p.ServiceName
	b.ServicePort = p.ServicePort
	b.Algorithm = a
	b.Endpoints = p.Endpoints
	b.parseError = perr

	*sb = b
	return nil
}

func (rg *RouteGroupSpec) UniqueHosts() []string {
	return uniqueStrings(rg.Hosts)
}

func hasEmpty(s []string) bool {
	for _, si := range s {
		if si == "" {
			return true
		}
	}

	return false
}

func (brs BackendReferences) validate(backends map[string]bool) error {
	if brs == nil {
		return nil
	}
	names := make(map[string]struct{}, len(brs))
	for _, br := range brs {
		if _, ok := names[br.BackendName]; ok {
			return duplicateBackendReference(br.BackendName)
		}
		names[br.BackendName] = struct{}{}

		if err := br.validate(backends); err != nil {
			return err
		}
	}
	return nil
}

func (br *BackendReference) validate(backends map[string]bool) error {
	if br == nil || br.BackendName == "" {
		return errUnnamedBackendReference
	}

	if !backends[br.BackendName] {
		return invalidBackendReference(br.BackendName)
	}

	if br.Weight < 0 {
		return invalidBackendWeight(br.BackendName, br.Weight)
	}

	return nil
}

func (r *RouteSpec) UniqueMethods() []string {
	return uniqueStrings(r.Methods)
}

// TODO: we need to pass namespace/name in all errors
func (rg *RouteGroupSpec) validate() error {
	// has at least one backend:
	if len(rg.Backends) == 0 {
		return errRouteGroupWithoutBackend
	}

	// backends valid and have unique names:
	backends := make(map[string]bool)
	for _, b := range rg.Backends {
		if backends[b.Name] {
			return backendsWithDuplicateName(b.Name)
		}

		backends[b.Name] = true
		if err := b.validate(); err != nil {
			return invalidBackend(b.Name, err)
		}
	}

	hasDefault := len(rg.DefaultBackends) > 0
	if err := rg.DefaultBackends.validate(backends); err != nil {
		return err
	}

	if !hasDefault && len(rg.Routes) == 0 {
		return errMissingBackendReference
	}

	for i, r := range rg.Routes {
		if err := r.validate(hasDefault, backends); err != nil {
			return invalidRoute(i, err)
		}
	}

	return nil
}

// TODO: we need to pass namespace/name in all errors
func (r *RouteSpec) validate(hasDefault bool, backends map[string]bool) error {
	if r == nil {
		return errInvalidRouteSpec
	}

	if !hasDefault && len(r.Backends) == 0 {
		return errMissingBackendReference
	}

	if err := r.Backends.validate(backends); err != nil {
		return err
	}

	if r.Path != "" && r.PathSubtree != "" {
		return errBothPathAndPathSubtree
	}

	if hasEmpty(r.Predicates) {
		return errInvalidPredicate
	}

	if hasEmpty(r.Filters) {
		return errInvalidFilter
	}

	if hasEmpty(r.Methods) {
		return errInvalidMethod
	}

	return nil
}

// TODO: we need to pass namespace/name in all errors
func (r *RouteGroupItem) validate() error {
	// has metadata and name:
	if r == nil || validate(r.Metadata) != nil {
		return errRouteGroupWithoutName
	}

	// has spec:
	if r.Spec == nil {
		return routeGroupError(r.Metadata, errRouteGroupWithoutSpec)
	}

	if err := r.Spec.validate(); err != nil {
		return routeGroupError(r.Metadata, err)
	}

	return nil
}

// ParseRouteGroupsJSON parses a json list of RouteGroups into RouteGroupList
func ParseRouteGroupsJSON(d []byte) (RouteGroupList, error) {
	var rl RouteGroupList
	err := json.Unmarshal(d, &rl)
	return rl, err
}

// ParseRouteGroupsYAML parses a YAML list of RouteGroups into RouteGroupList
func ParseRouteGroupsYAML(d []byte) (RouteGroupList, error) {
	var rl RouteGroupList
	err := yaml.Unmarshal(d, &rl)
	return rl, err
}

// ValidateRouteGroup validates a RouteGroupItem
func ValidateRouteGroup(rg *RouteGroupItem) error {
	return rg.validate()
}

// ValidateRouteGroups validates a RouteGroupList
func ValidateRouteGroups(rl *RouteGroupList) error {
	var err error
	// avoid the user having to repeatedly validate to discover all errors
	for _, i := range rl.Items {
		nerr := ValidateRouteGroup(i)
		if nerr != nil {
			err = errors.Wrap(err, nerr.Error())
		}
	}

	if err != nil {
		return err
	}

	return nil
}
