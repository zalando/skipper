package routegroup

import (
	"encoding/json"
	"fmt"

	"github.com/prometheus/common/log"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/zalando/skipper/dataclients/kubernetes"
)

var (
	scheme       = runtime.NewScheme()
	codecs       = serializer.NewCodecFactory(scheme)
	deserializer = codecs.UniversalDeserializer()
)

func admit(ar *v1beta1.AdmissionRequest) *v1beta1.AdmissionResponse {
	rgItem := kubernetes.RouteGroupItem{}
	err := json.Unmarshal(ar.Object.Raw, &rgItem)
	if err != nil {
		emsg := fmt.Errorf("could not parse RouteGroup, %w", err)
		log.Error(emsg)
		return &v1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: emsg.Error(),
			},
		}
	}

	err = rgItem.Validate()
	if err != nil {
		emsg := fmt.Errorf("could not validate RouteGroup, %w", err)
		log.Error(emsg)
		return &v1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: emsg.Error(),
			},
		}
	}

	return &v1beta1.AdmissionResponse{
		Allowed: true,
	}
}
