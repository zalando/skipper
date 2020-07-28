package admission

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	admissionsv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	MetricNamespace = "routegroup_admission"
	metricSubsystem = "admitter"
)

var (
	labels = []string{"admitter", "operation", "group", "version", "resource", "sub_resource"}

	totalRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: MetricNamespace,
		Subsystem: metricSubsystem,
		Name:      "requests",
		Help:      "Total number of requests to this admitter",
	}, []string{"admitter"})
	invalidRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: MetricNamespace,
		Subsystem: metricSubsystem,
		Name:      "invalid_requests",
		Help:      "Total number of requests to this admitter that couldn't be parsed",
	}, []string{"admitter"})
	rejectedRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: MetricNamespace,
		Subsystem: metricSubsystem,
		Name:      "rejected_admissions",
		Help:      "Total number of requests rejected by this admitter",
	}, labels)
	approvedRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: MetricNamespace,
		Subsystem: metricSubsystem,
		Name:      "successful_admissions",
		Help:      "Total number of requests successfully processed by this admitter",
	}, labels)
	admissionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: MetricNamespace,
		Subsystem: metricSubsystem,
		Name:      "admission_duration",
		Help:      "Duration of admission calls",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, .75, 1, 1.25, 1.5, 2, 2.5, 5, 10},
	}, labels)
)

type Admitter interface {
	Name() string
	Admit(req *admissionsv1.AdmissionRequest) (*admissionsv1.AdmissionResponse, error)
}

type RouteGroupAdmitter struct {
}

func init() {
	prometheus.MustRegister(totalRequests, invalidRequests, rejectedRequests, approvedRequests, admissionDuration)
}

func (r RouteGroupAdmitter) Name() string {
	return "routegroup"
}

func (r RouteGroupAdmitter) Admit(req *admissionsv1.AdmissionRequest) (*admissionsv1.AdmissionResponse, error) {
	rgItem := definitions.RouteGroupItem{}
	err := json.Unmarshal(req.Object.Raw, &rgItem)
	if err != nil {
		emsg := fmt.Errorf("could not parse RouteGroup, %w", err)
		log.Error(emsg)
		return &admissionsv1.AdmissionResponse{
			UID:     req.UID,
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
			UID:     req.UID,
			Allowed: false,
			Result: &metav1.Status{
				Message: emsg.Error(),
			},
		}, nil
	}

	return &admissionsv1.AdmissionResponse{
		UID:     req.UID,
		Allowed: true,
	}, nil
}

func Handler(admitter Admitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admitterName := admitter.Name()
		totalRequests.WithLabelValues(admitterName).Inc()

		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			invalidRequests.WithLabelValues(admitterName).Inc()
			w.WriteHeader(http.StatusBadRequest)
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Errorf("Unable to read request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			invalidRequests.WithLabelValues(admitterName).Inc()
			return
		}

		review := admissionsv1.AdmissionReview{}
		err = json.Unmarshal(body, &review)
		if err != nil {
			log.Errorf("Unable to parse request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			invalidRequests.WithLabelValues(admitterName).Inc()
			return
		}

		operationInfo := fmt.Sprintf(
			"%s %s %s/%s",
			review.Request.Operation,
			review.Request.Kind,
			review.Request.Namespace,
			extractName(review.Request),
		)

		gvr := review.Request.Resource
		group := gvr.Group
		if group == "" {
			group = "zalando.org"
		}

		labelValues := prometheus.Labels{
			"admitter":     admitterName,
			"operation":    string(review.Request.Operation),
			"group":        group,
			"version":      gvr.Version,
			"resource":     gvr.Resource,
			"sub_resource": review.Request.SubResource,
		}

		start := time.Now()
		defer admissionDuration.With(labelValues).
			Observe(float64(time.Since(start)) / float64(time.Second))

		admResp, err := admitter.Admit(review.Request)
		if err != nil {
			log.Errorf("Rejected %s: %v", operationInfo, err)
			writeResponse(w, errorResponse(review.Request.UID, err))
			rejectedRequests.With(labelValues).Inc()
			return
		}

		log.Infof("Allowed %s", operationInfo)
		approvedRequests.With(labelValues).Inc()
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

func errorResponse(uid types.UID, err error) *admissionsv1.AdmissionResponse {
	return &admissionsv1.AdmissionResponse{
		Allowed: false,
		UID:     uid,
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func extractName(request *admissionsv1.AdmissionRequest) string {
	if request.Name != "" {
		return request.Name
	}

	obj := metav1.PartialObjectMetadata{}
	if err := json.Unmarshal(request.Object.Raw, &obj); err != nil {
		log.Warnf("unable to parse object: %v", err)
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
