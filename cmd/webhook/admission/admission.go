package admission

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"
	admissionsv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type admitter interface {
	Admit(req *admissionsv1.AdmissionRequest) (*admissionsv1.AdmissionResponse, error)
}

type routegroupAdmitter struct {
}

func (r routegroupAdmitter) Admit(req *admissionsv1.AdmissionRequest) (*admissionsv1.AdmissionResponse, error) {
	rgItem := definitions.RouteGroupItem{}
	err := json.Unmarshal(req.Object.Raw, &rgItem)
	if err != nil {
		emsg := fmt.Errorf("could not parse RouteGroup, %w", err)
		log.Error(emsg)
		return &admissionsv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: emsg.Error(),
			},
		}, nil
	}

	err = definitions.ValidateRouteGroup(&rgItem)
	if err != nil {
		emsg := fmt.Errorf("could not validate RouteGroup, %w", err)
		log.Error(emsg)
		return &admissionsv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: emsg.Error(),
			},
		}, nil
	}

	return &admissionsv1.AdmissionResponse{
		Allowed: true,
	}, nil
}

func Handler(admitter admitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			// TODO: inc prometheus invalid req counter
			w.WriteHeader(http.StatusBadRequest)
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Errorf("Unable to read request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			// TODO: inc prometheus invalid req counter
			return
		}

		review := admissionsv1.AdmissionReview{}
		err = json.Unmarshal(body, &review)
		if err != nil {
			log.Errorf("Unable to parse request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			// TODO: inc prom counter
			//invalidRequests.WithLabelValues(admitterName).Inc()
			return
		}

		admResp, err := admitter.Admit(review.Request)
		if err != nil {
			// TODO: add op info
			log.Errorf("Rejected: %v", err)
			if _, err := w.Write([]byte("ok")); err != nil {
				log.Errorf("unable to write response: %v", err)
			}
			// TODO: inc prom rejectedRequests counter
			return
		}

		// TODO: match UID of requests with response
		writeResponse(w, admResp)
	}
}

func writeResponse(writer http.ResponseWriter, response *admissionsv1.AdmissionResponse) {
	resp, err := json.Marshal(admissionsv1.AdmissionReview{
		Response: response,
	})
	if err != nil {
		log.Errorf("unable to serialize response: %v", err)
		writer.WriteHeader(http.StatusInternalServerError)
	}
	if _, err := writer.Write(resp); err != nil {
		log.Errorf("unable to write response: %v", err)
	}
}
