package definitions

import (
	"github.com/zalando/skipper/eskip"
)

type RouteGroupValidator struct{}

var defaultRouteGroupValidator = &RouteGroupValidator{}

// ValidateRouteGroup validates a RouteGroupItem
func ValidateRouteGroup(rg *RouteGroupItem) error {
	return defaultRouteGroupValidator.Validate(rg)
}

func ValidateRouteGroups(rgl *RouteGroupList) error {
	var err []error
	for _, rg := range rgl.Items {
		// TODO: Use errors.Join
		err = append(err, defaultRouteGroupValidator.Validate(rg))
	}
	return errorsJoin(err...)
}

func (rgv *RouteGroupValidator) Validate(item *RouteGroupItem) error {
	e := rgv.basicValidation(item)
	if e != nil {
		return e
	}
	var err []error
	err = append(err, rgv.filtersValidation(item))
	err = append(err, rgv.predicatesValidation(item))

	return errorsJoin(err...)
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

func (rgv *RouteGroupValidator) filtersValidation(item *RouteGroupItem) error {
	var err []error
	for _, r := range item.Spec.Routes {
		for _, f := range r.Filters {
			_, e := eskip.ParseFilters(f)
			// TODO: Use errors.Join
			err = append(err, e)
		}
	}

	return errorsJoin(err...)
}

func (rgv *RouteGroupValidator) predicatesValidation(item *RouteGroupItem) error {
	var err []error
	for _, r := range item.Spec.Routes {
		for _, p := range r.Predicates {
			_, e := eskip.ParsePredicates(p)
			// TODO: Use errors.Join
			err = append(err, e)
		}
	}
	return errorsJoin(err...)
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
