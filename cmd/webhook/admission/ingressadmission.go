package admission

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type IngressAdmitter struct {
}

func (IngressAdmitter) name() string {
	return "ingress"
}

func (IngressAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {
	ingressItem := definitions.IngressV1Item{}
	err := json.Unmarshal(req.Object, &ingressItem)
	if err != nil {
		emsg := fmt.Sprintf("could not parse Ingress, %v", err)
		log.Error(emsg)
		return &admissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &status{
				Message: emsg,
			},
		}, nil
	}

	err = definitions.ValidateIngressV1(&ingressItem)
	if err != nil {
		emsg := fmt.Sprintf("Ingress validation failed, %v", err)
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
