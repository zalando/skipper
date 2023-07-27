package definitions

import (
	"encoding/json"
	"fmt"

	"errors"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/loadbalancer"
	"gopkg.in/yaml.v2"
)

type RouteGroupValidator struct {
	FilterRegistry filters.Registry
}

var rgv = &RouteGroupValidator{}

func (rgv *RouteGroupValidator) Validate(item *RouteGroupItem) error {
	err := rgv.BasicValidation(item)
	if rgv.FilterRegistry != nil {
		err = errors.Join(err, rgv.FiltersValidation(item))
	}
	return err
}

func (rgv *RouteGroupValidator) BasicValidation(item *RouteGroupItem) error {
	return item.validate()
}

func (rgv *RouteGroupValidator) FiltersValidation(item *RouteGroupItem) error {
	return validateRouteGroupFilters(item, rgv.FilterRegistry)
}

func (rgv *RouteGroupValidator) ValidateList(rl *RouteGroupList) error {
	err := rgv.BasicValidationList(rl)

	if rgv.FilterRegistry != nil {
		err = errors.Join(err, rgv.FiltersValidationList(rl))
	}
	return err
}

func (rgv *RouteGroupValidator) BasicValidationList(rl *RouteGroupList) error {
	var err error
	// avoid the user having to repeatedly validate to discover all errors
	for _, i := range rl.Items {
		err = errors.Join(err, rgv.BasicValidation(i))
	}
	return err
}

func (rgv *RouteGroupValidator) FiltersValidationList(rl *RouteGroupList) error {
	var err error
	// avoid the user having to repeatedly validate to discover all errors
	for _, i := range rl.Items {
		err = errors.Join(err, rgv.FiltersValidation(i))
	}
	return err
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

func (r *RouteSpec) UniqueMethods() []string {
	return uniqueStrings(r.Methods)
}

// validateRouteGroup validates a RouteGroupItem
func ValidateRouteGroup(rg *RouteGroupItem) error {
	return rgv.Validate(rg)
}

// ValidateRouteGroups validates a RouteGroupList
func ValidateRouteGroups(rl *RouteGroupList) error {
	return rgv.ValidateList(rl)
}

func validateRouteGroupFilters(rg *RouteGroupItem, fr filters.Registry) error {
	// basic for now
	for _, r := range rg.Spec.Routes {
		for _, f := range r.Filters {
			parsedFilter, err := eskip.ParseFilters(f)
			if err != nil {
				return err
			}
			if _, ok := fr[parsedFilter[0].Name]; !ok {
				return fmt.Errorf("filter %q not found", parsedFilter[0].Name)
			}
		}
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

func hasEmpty(s []string) bool {
	for _, si := range s {
		if si == "" {
			return true
		}
	}

	return false
}
