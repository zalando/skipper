package admission

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const (
	metricNamespace = "routegroup_admission"
	metricSubsystem = "admitter"
)

var (
	labels = []string{"admitter", "operation", "group", "version", "resource", "sub_resource"}

	totalRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Subsystem: metricSubsystem,
		Name:      "requests",
		Help:      "Total number of requests to this admitter",
	}, []string{"admitter"})
	invalidRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Subsystem: metricSubsystem,
		Name:      "invalid_requests",
		Help:      "Total number of requests to this admitter that couldn't be parsed",
	}, []string{"admitter"})
	rejectedRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Subsystem: metricSubsystem,
		Name:      "rejected_admissions",
		Help:      "Total number of requests rejected by this admitter",
	}, labels)
	approvedRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricNamespace,
		Subsystem: metricSubsystem,
		Name:      "successful_admissions",
		Help:      "Total number of requests successfully processed by this admitter",
	}, labels)
	admissionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Subsystem: metricSubsystem,
		Name:      "admission_duration",
		Help:      "Duration of admission calls",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, .75, 1, 1.25, 1.5, 2, 2.5, 5, 10},
	}, labels)
)

type admitter interface {
	name() string
	admit(req *admissionRequest) (*admissionResponse, error)
}

func init() {
	prometheus.MustRegister(totalRequests, invalidRequests, rejectedRequests, approvedRequests, admissionDuration)
}

func Handler(admitter admitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admitterName := admitter.name()
		totalRequests.WithLabelValues(admitterName).Inc()

		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			invalidRequests.WithLabelValues(admitterName).Inc()
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Errorf("Failed to read request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			invalidRequests.WithLabelValues(admitterName).Inc()
			return
		}

		var review admissionReview
		err = json.Unmarshal(body, &review)
		if err != nil {
			log.Errorf("Failed to parse request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			invalidRequests.WithLabelValues(admitterName).Inc()
			return
		}

		request := review.Request
		if request == nil {
			log.Errorf("Missing review request")
			w.WriteHeader(http.StatusBadRequest)
			invalidRequests.WithLabelValues(admitterName).Inc()
			return
		}

		log := log.WithFields(log.Fields{
			"operation": request.Operation,
			"kind":      request.Kind,
			"namespace": request.Namespace,
			"name":      extractName(request),
			"user":      request.UserInfo.Username,
		})

		gvr := request.Resource
		group := gvr.Group
		if group == "" {
			group = "zalando.org"
		}

		labelValues := prometheus.Labels{
			"admitter":     admitterName,
			"operation":    request.Operation,
			"group":        group,
			"version":      gvr.Version,
			"resource":     gvr.Resource,
			"sub_resource": request.SubResource,
		}

		start := time.Now()
		defer func() {
			admissionDuration.With(labelValues).Observe(float64(time.Since(start)) / float64(time.Second))
		}()

		admResp, err := admitter.admit(request)
		if err != nil {
			log.Errorf("Failed to admit request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			invalidRequests.WithLabelValues(admitterName).Inc()
			return
		}

		if admResp.Allowed {
			log.Debugf("Allowed")
			approvedRequests.With(labelValues).Inc()
		} else {
			log.Debugf("Rejected")
			rejectedRequests.With(labelValues).Inc()
		}

		writeResponse(w, admResp)
	}
}

func writeResponse(writer http.ResponseWriter, response *admissionResponse) {
	resp, err := json.Marshal(admissionReview{
		typeMeta: typeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: response,
	})
	if err != nil {
		log.Errorf("failed to serialize response: %v", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := writer.Write(resp); err != nil {
		log.Errorf("failed to write response: %v", err)
	}
}

func extractName(request *admissionRequest) string {
	if request.Name != "" {
		return request.Name
	}

	obj := partialObjectMetadata{}
	if err := json.Unmarshal(request.Object, &obj); err != nil {
		log.Warnf("failed to parse object: %v", err)
		return "<unknown>"
	}

	if obj.Name != "" {
		return obj.Name
	}
	if obj.GenerateName != "" {
		return obj.GenerateName + "<generated>"
	}
	return "<unknown>"
}
