package admission

import (
	"encoding/json"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type IngressAdmitter struct {
	IngressValidator *definitions.IngressV1Validator
}

func (iga *IngressAdmitter) name() string {
	return "ingress"
}

func (iga *IngressAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {

	ingressItem := definitions.IngressV1Item{}
	err := json.Unmarshal(req.Object, &ingressItem)
	if err != nil {
		return nil, err
	}

	err = iga.IngressValidator.Validate(&ingressItem)
	if err != nil {
		return nil, err
	}

	return &admissionResponse{
		UID:     req.UID,
		Allowed: true,
	}, nil
}
