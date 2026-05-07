package metrics

import (
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

type All struct {
	providers         []Metrics
	prometheus        *Prometheus
	codaHale          *CodaHale
	otel              *OTel
	prometheusHandler http.Handler
	codaHaleHandler   http.Handler
}

func NewAll(o Options) *All {
	all := &All{
		codaHale:   NewCodaHale(o),
		prometheus: NewPrometheus(o),
	}
	providers := make([]Metrics, 0)
	providers = append(providers, all.codaHale, all.prometheus)

	otel, err := NewOTel(o)
	if err != nil {
		logrus.Errorf("Failed to crete otel: %v", err)
	} else {
		all.otel = otel
		providers = append(providers, otel)
	}

	all.providers = providers

	return all
}

func (a *All) MeasureSince(key string, start time.Time) {
	for _, p := range a.providers {
		p.MeasureSince(key, start)
	}
}

func (a *All) IncCounter(key string) {
	for _, p := range a.providers {
		p.IncCounter(key)
	}
}

func (a *All) IncCounterBy(key string, value int64) {
	for _, p := range a.providers {
		p.IncCounterBy(key, value)
	}
}

func (a *All) IncFloatCounterBy(key string, value float64) {
	for _, p := range a.providers {
		p.IncFloatCounterBy(key, value)
	}
}

func (a *All) UpdateGauge(key string, v float64) {
	for _, p := range a.providers {
		p.UpdateGauge(key, v)
	}
}

func (a *All) MeasureRouteLookup(start time.Time) {
	for _, p := range a.providers {
		p.MeasureRouteLookup(start)
	}
}

func (a *All) MeasureFilterCreate(filterName string, start time.Time) {
	for _, p := range a.providers {
		p.MeasureFilterCreate(filterName, start)
	}
}

func (a *All) MeasureFilterRequest(filterName string, start time.Time) {
	for _, p := range a.providers {
		p.MeasureFilterRequest(filterName, start)
	}
}

func (a *All) MeasureAllFiltersRequest(routeId string, start time.Time) {
	for _, p := range a.providers {
		p.MeasureAllFiltersRequest(routeId, start)
	}
}

func (a *All) MeasureBackendRequestHeader(host string, size int) {
	for _, p := range a.providers {
		p.MeasureBackendRequestHeader(host, size)
	}
}

func (a *All) MeasureBackend(routeId string, start time.Time) {
	for _, p := range a.providers {
		p.MeasureBackend(routeId, start)
	}
}

func (a *All) MeasureBackendHost(routeBackendHost string, start time.Time) {
	for _, p := range a.providers {
		p.MeasureBackendHost(routeBackendHost, start)
	}
}

func (a *All) MeasureFilterResponse(filterName string, start time.Time) {
	for _, p := range a.providers {
		p.MeasureFilterResponse(filterName, start)
	}
}

func (a *All) MeasureAllFiltersResponse(routeId string, start time.Time) {
	for _, p := range a.providers {
		p.MeasureAllFiltersResponse(routeId, start)
	}
}

func (a *All) MeasureResponse(code int, method string, routeId string, start time.Time) {
	for _, p := range a.providers {
		p.MeasureResponse(code, method, routeId, start)
	}
}

func (a *All) MeasureResponseSize(host string, size int64) {
	for _, p := range a.providers {
		p.MeasureResponseSize(host, size)
	}
}

func (a *All) MeasureProxy(requestDuration, responseDuration time.Duration) {
	for _, p := range a.providers {
		p.MeasureProxy(requestDuration, responseDuration)
	}
}

func (a *All) MeasureServe(routeId, host, method string, code int, start time.Time) {
	for _, p := range a.providers {
		p.MeasureServe(routeId, host, method, code, start)
	}
}

func (a *All) IncRoutingFailures() {
	for _, p := range a.providers {
		p.IncRoutingFailures()
	}
}

func (a *All) IncErrorsBackend(routeId string) {
	for _, p := range a.providers {
		p.IncErrorsBackend(routeId)
	}
}

func (a *All) MeasureBackend5xx(t time.Time) {
	for _, p := range a.providers {
		p.MeasureBackend5xx(t)
	}
}

func (a *All) IncErrorsStreaming(routeId string) {
	for _, p := range a.providers {
		p.IncErrorsStreaming(routeId)
	}

}

func (a *All) SetInvalidRoute(routeId, reason string) {
	for _, p := range a.providers {
		p.SetInvalidRoute(routeId, reason)
	}
}

func (a *All) Close() {
	for _, p := range a.providers {
		p.Close()
	}
}

func (a *All) String() string {
	res := []string{}
	for _, p := range a.providers {
		res = append(res, p.String())
	}
	return strings.Join(res, "|")
}

// ScopedPrometheusRegisterer implements the PrometheusMetrics interface
func (a *All) ScopedPrometheusRegisterer(subsystem string) prometheus.Registerer {
	return a.prometheus.ScopedPrometheusRegisterer(subsystem)
}

func (a *All) RegisterHandler(path string, handler *http.ServeMux) {
	if a.prometheus != nil {
		a.prometheusHandler = a.prometheus.getHandler()
	} else {
		a.prometheusHandler = voidHandler{}
	}
	if a.codaHale != nil {
		a.codaHaleHandler = a.codaHale.getHandler(path)
	} else {
		a.codaHaleHandler = voidHandler{}
	}
	handler.Handle(path, a.newHandler())
}

func (a *All) newHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Accept") == "application/codahale+json" {
			a.codaHaleHandler.ServeHTTP(w, req)
		} else {
			a.prometheusHandler.ServeHTTP(w, req)
		}
	})
}

type voidHandler struct{}

func (voidHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(http.StatusText(http.StatusNotFound)))
}
