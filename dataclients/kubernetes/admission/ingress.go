package admission

import (
	"encoding/json"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type IngressAdmitter struct {
	IngressValidator definitions.Validator[*definitions.IngressV1Item]
}

func (iga *IngressAdmitter) name() string {
	return "ingress"
}

func (iga *IngressAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {
	var ingressItem definitions.IngressV1Item
	if err := json.Unmarshal(req.Object, &ingressItem); err != nil {
		return nil, err
	}

	if err := iga.IngressValidator.Validate(&ingressItem); err != nil {
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
