package admission

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type RouteGroupAdmitter struct {
}

func (RouteGroupAdmitter) name() string {
	return "routegroup"
}

func (RouteGroupAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {
	rgItem := definitions.RouteGroupItem{}
	err := json.Unmarshal(req.Object, &rgItem)
	if err != nil {
		emsg := fmt.Sprintf("Could not parse RouteGroup: %v", err)
		log.Error(emsg)
		return &admissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &status{
				Message: emsg,
			},
		}, nil
	}

	err = definitions.ValidateRouteGroup(&rgItem)
	if err != nil {
		emsg := fmt.Sprintf("Could not validate RouteGroup: %v", err)
		log.Error(emsg)
		return &admissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &status{
				Message: emsg,
			},
		}, nil
	}

	return &admissionResponse{
		UID:     req.UID,
		Allowed: true,
	}, nil
}
