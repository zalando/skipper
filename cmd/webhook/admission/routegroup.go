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

	rgItem := definitions.RouteGroupItem{}
	err := json.Unmarshal(req.Object, &rgItem)
	if err != nil {
		return nil, err
	}

	err = rga.RouteGroupValidator.Validate(&rgItem)
	if err != nil {
		return nil, err
	}

	return &admissionResponse{
		UID:     req.UID,
		Allowed: true,
	}, nil
}
