package admission

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
)

type admitter interface {
	Admit(req *admissionv1.AdmissionRequest) (admissionv1.AdmissionResponse, error)
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

		review := admissionv1.AdmissionReview{}
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

		writeResponse(w, &admResp)
	}
}

func writeResponse(writer http.ResponseWriter, response *admissionv1.AdmissionResponse) {
	resp, err := json.Marshal(admissionv1.AdmissionReview{
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
