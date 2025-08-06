package admission

import (
	"encoding/json"
	"fmt"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/internal/validation"
	"github.com/zalando/skipper/routing"
)

type IngressAdmitter struct {
	validator *validation.ComprehensiveValidator
	// Backward compatibility field
	IngressValidator *definitions.IngressV1Validator
}

func NewIngressAdmitter(filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec) *IngressAdmitter {
	return &IngressAdmitter{
		validator: validation.NewComprehensiveValidator(filterRegistry, predicateSpecs),
	}
}

func (ia *IngressAdmitter) name() string {
	return "ingress-admitter"
}

func (ia *IngressAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {
	switch req.Operation {
	case "CREATE", "UPDATE":
		var ing definitions.IngressV1Item
		if err := json.Unmarshal(req.Object, &ing); err != nil {
			return &admissionResponse{
				UID:     req.UID,
				Allowed: false,
				Result: &status{
					Message: fmt.Sprintf("Failed to parse Ingress: %v", err),
				},
			}, nil
		}

		// Use old validator if present (backward compatibility)
		if ia.IngressValidator != nil {
			if err := ia.IngressValidator.Validate(&ing); err != nil {
				return &admissionResponse{
					UID:     req.UID,
					Allowed: false,
					Result: &status{
						Message: fmt.Sprintf("Ingress validation failed: %v", err),
					},
				}, nil
			}
		} else if ia.validator != nil {
			// Use new comprehensive validator
			if err := ia.validator.ValidateIngress(&ing); err != nil {
				return &admissionResponse{
					UID:     req.UID,
					Allowed: false,
					Result: &status{
						Message: fmt.Sprintf("Ingress validation failed: %v", err),
					},
				}, nil
			}
		}

		return &admissionResponse{UID: req.UID, Allowed: true}, nil

	default:
		return &admissionResponse{UID: req.UID, Allowed: true}, nil
	}
}
