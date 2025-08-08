package admission

import (
	"encoding/json"
	"fmt"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/internal/validation"
	"github.com/zalando/skipper/routing"
)

type RouteGroupAdmitter struct {
	validator *validation.ComprehensiveValidator
	// Backward compatibility field
	RouteGroupValidator *definitions.RouteGroupValidator
}

func NewRouteGroupAdmitter(filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec) *RouteGroupAdmitter {
	return &RouteGroupAdmitter{
		validator: validation.NewComprehensiveValidator(filterRegistry, predicateSpecs),
	}
}

func (rga *RouteGroupAdmitter) name() string {
	return "routegroup-admitter"
}

func (rga *RouteGroupAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {
	switch req.Operation {
	case "CREATE", "UPDATE":
		var rg definitions.RouteGroupItem
		if err := json.Unmarshal(req.Object, &rg); err != nil {
			return &admissionResponse{
				UID:     req.UID,
				Allowed: false,
				Result: &status{
					Message: fmt.Sprintf("Failed to parse RouteGroup: %v", err),
				},
			}, nil
		}

		// Use old validator if present (backward compatibility)
		if rga.RouteGroupValidator != nil {
			if err := rga.RouteGroupValidator.Validate(&rg); err != nil {
				return &admissionResponse{
					UID:     req.UID,
					Allowed: false,
					Result: &status{
						Message: fmt.Sprintf("RouteGroup validation failed: %v", err),
					},
				}, nil
			}
		} else if rga.validator != nil {
			// Use new comprehensive validator
			if err := rga.validator.ValidateRouteGroup(&rg); err != nil {
				return &admissionResponse{
					UID:     req.UID,
					Allowed: false,
					Result: &status{
						Message: fmt.Sprintf("RouteGroup validation failed: %v", err),
					},
				}, nil
			}
		}

		return &admissionResponse{UID: req.UID, Allowed: true}, nil

	default:
		return &admissionResponse{UID: req.UID, Allowed: true}, nil
	}
}
