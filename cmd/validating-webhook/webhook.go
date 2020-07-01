package validating_webhook

import (
	"encoding/json"

	"github.com/prometheus/common/log"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validate(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request
	var rg kubernetes.RouteGroupItem
	if err := json.Unmarshal(req.Object.Raw, &rg); err != nil {
		log.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	return &v1beta1.AdmissionResponse{
		Allowed: true,
	}
}
