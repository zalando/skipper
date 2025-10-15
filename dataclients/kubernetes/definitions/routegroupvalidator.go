package definitions

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

type RouteGroupValidator struct {
	FilterRegistry           filters.Registry
	PredicateSpecs           []routing.PredicateSpec
	Metrics                  metrics.Metrics
	EnableAdvancedValidation bool
}

// check if RouteGroupValidator implements the interface
var _ Validator[*RouteGroupItem] = &RouteGroupValidator{}

var (
	errSingleFilterExpected    = errors.New("single filter expected")
	errSinglePredicateExpected = errors.New("single predicate expected")
)

var defaultRouteGroupValidator = &RouteGroupValidator{}

// ValidateRouteGroup validates a RouteGroupItem
func ValidateRouteGroup(rg *RouteGroupItem) error {
	return defaultRouteGroupValidator.Validate(rg)
}

func ValidateRouteGroups(rgl *RouteGroupList) error {
	var errs []error
	for _, rg := range rgl.Items {
		errs = append(errs, defaultRouteGroupValidator.Validate(rg))
	}
	return errors.Join(errs...)
}

func (rgv *RouteGroupValidator) Validate(item *RouteGroupItem) error {
	err := rgv.basicValidation(item)
	if err != nil {
		return err
	}
	var errs []error
	errs = append(errs, rgv.validateFilters(item))
	errs = append(errs, rgv.validatePredicates(item))
	errs = append(errs, rgv.validateBackends(item))
	errs = append(errs, rgv.validateHosts(item))

	return errors.Join(errs...)
}

// TODO: we need to pass namespace/name in all errors
func (rgv *RouteGroupValidator) basicValidation(r *RouteGroupItem) error {
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

func (rgv *RouteGroupValidator) validateFilters(item *RouteGroupItem) error {
	var errs []error
	for _, r := range item.Spec.Routes {
		for _, f := range r.Filters {
			parsedFilters, err := eskip.ParseFilters(f)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if len(parsedFilters) != 1 {
				errs = append(errs, fmt.Errorf("%w at %q", errSingleFilterExpected, f))
				continue
			}
			if rgv.EnableAdvancedValidation {
				if err := validateFilters(ResourceContext{
					Namespace:    item.Metadata.Namespace,
					Name:         item.Metadata.Name,
					ResourceType: ResourceTypeRouteGroup,
				}, rgv.FilterRegistry, rgv.Metrics, parsedFilters); err != nil {
					errs = append(errs, fmt.Errorf("invalid filter %q: %w", f, err))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (rgv *RouteGroupValidator) validatePredicates(item *RouteGroupItem) error {
	var errs []error
	for _, r := range item.Spec.Routes {
		routePredicates := make([]*eskip.Predicate, 0, len(r.Predicates))
		for _, p := range r.Predicates {
			parsedPredicates, err := eskip.ParsePredicates(p)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if len(parsedPredicates) != 1 {
				errs = append(errs, fmt.Errorf("%w at %q", errSinglePredicateExpected, p))
				continue
			}
			routePredicates = append(routePredicates, parsedPredicates[0])
		}
		if rgv.EnableAdvancedValidation {
			if err := validatePredicates(ResourceContext{
				Namespace:    item.Metadata.Namespace,
				Name:         item.Metadata.Name,
				ResourceType: ResourceTypeRouteGroup,
			}, rgv.PredicateSpecs, rgv.Metrics, routePredicates); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (rgv *RouteGroupValidator) validateBackends(item *RouteGroupItem) error {
	var errs []error
	for _, backend := range item.Spec.Backends {
		if backend.Type != eskip.NetworkBackend {
			continue
		}
		address, err := url.Parse(backend.Address)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to parse backend address %q: %w", backend.Address, err))
			continue
		}
		if address.Path != "" || address.RawQuery != "" || address.Scheme == "" || address.Host == "" {
			errs = append(errs, fmt.Errorf("backend address %q does not match scheme://host format", backend.Address))
			continue
		}
		if rgv.EnableAdvancedValidation {
			if err := validateBackend(ResourceContext{
				Namespace:    item.Metadata.Namespace,
				Name:         item.Metadata.Name,
				ResourceType: ResourceTypeRouteGroup,
			}, backend.Address, backend.Type, rgv.Metrics); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (rgv *RouteGroupValidator) validateHosts(item *RouteGroupItem) error {
	var errs []error
	uniqueHosts := make(map[string]struct{}, len(item.Spec.Hosts))
	for _, host := range item.Spec.Hosts {
		if _, ok := uniqueHosts[host]; ok {
			errs = append(errs, fmt.Errorf("duplicate host %q", host))
		}
		uniqueHosts[host] = struct{}{}
	}
	return errors.Join(errs...)
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
