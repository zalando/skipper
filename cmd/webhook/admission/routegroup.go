package admission

import (
	"encoding/json"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type RouteGroupAdmitter struct {
	RouteGroupValidator *definitions.RouteGroupValidator
}

func (rga *RouteGroupAdmitter) name() string {
	return "routegroup"
}

func (rga *RouteGroupAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {

	// Serve as default validator if not set
	if rga.RouteGroupValidator == nil {
		rga.RouteGroupValidator = &definitions.RouteGroupValidator{}
	}

	rgItem := definitions.RouteGroupItem{}
	err := json.Unmarshal(req.Object, &rgItem)
	if err != nil {
		return &admissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &status{
				Message: err.Error(),
			},
		}, err
	}

	err = rga.RouteGroupValidator.Validate(&rgItem)
	if err != nil {
		return &admissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &status{
				Message: err.Error(),
			},
		}, err
	}

	return &admissionResponse{
		UID:     req.UID,
		Allowed: true,
	}, nil
}
