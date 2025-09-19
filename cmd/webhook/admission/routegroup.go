package admission

import (
	"encoding/json"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type RouteGroupAdmitter struct {
	RouteGroupValidator definitions.Validator[*definitions.RouteGroupItem]
}

func (rga *RouteGroupAdmitter) name() string {
	return "routegroup"
}

func (rga *RouteGroupAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {
	var rgItem definitions.RouteGroupItem
	if err := json.Unmarshal(req.Object, &rgItem); err != nil {
		return nil, err
	}

	if err := rga.RouteGroupValidator.Validate(&rgItem); err != nil {
		return &admissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result:  &status{Message: err.Error()},
		}, nil
	}

	return &admissionResponse{
		UID:     req.UID,
		Allowed: true,
	}, nil
}
