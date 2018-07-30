package metrics

import (
	"net/http"
	"time"
)

type All struct {
	prometheus        *Prometheus
	codaHale          *CodaHale
	prometheusHandler http.Handler
	codaHaleHandler   http.Handler
}

func NewAll(o Options) *All {
	return &All{
		prometheus: NewPrometheus(o),
		codaHale:   NewCodaHale(o),
	}
}

func (a *All) MeasureSince(key string, start time.Time) {
	a.prometheus.MeasureSince(key, start)
	a.codaHale.MeasureSince(key, start)
}
func (a *All) IncCounter(key string) {
	a.prometheus.IncCounter(key)
	a.codaHale.IncCounter(key)

}
func (a *All) UpdateGauge(key string, v float64) {
	a.prometheus.UpdateGauge(key, v)
	a.codaHale.UpdateGauge(key, v)
}
func (a *All) MeasureRouteLookup(start time.Time) {
	a.prometheus.MeasureRouteLookup(start)
	a.codaHale.MeasureRouteLookup(start)
}
func (a *All) MeasureFilterRequest(filterName string, start time.Time) {
	a.prometheus.MeasureFilterRequest(filterName, start)
	a.codaHale.MeasureFilterRequest(filterName, start)
}
func (a *All) MeasureAllFiltersRequest(routeId string, start time.Time) {
	a.prometheus.MeasureAllFiltersRequest(routeId, start)
	a.codaHale.MeasureAllFiltersRequest(routeId, start)
}
func (a *All) MeasureBackend(routeId string, start time.Time) {
	a.prometheus.MeasureBackend(routeId, start)
	a.codaHale.MeasureBackend(routeId, start)
}
func (a *All) MeasureBackendHost(routeBackendHost string, start time.Time) {
	a.prometheus.MeasureBackendHost(routeBackendHost, start)
	a.codaHale.MeasureBackendHost(routeBackendHost, start)
}
func (a *All) MeasureFilterResponse(filterName string, start time.Time) {
	a.prometheus.MeasureFilterResponse(filterName, start)
	a.codaHale.MeasureFilterResponse(filterName, start)
}
func (a *All) MeasureAllFiltersResponse(routeId string, start time.Time) {
	a.prometheus.MeasureAllFiltersResponse(routeId, start)
	a.codaHale.MeasureAllFiltersResponse(routeId, start)
}
func (a *All) MeasureResponse(code int, method string, routeId string, start time.Time) {
	a.prometheus.MeasureResponse(code, method, routeId, start)
	a.codaHale.MeasureResponse(code, method, routeId, start)
}
func (a *All) MeasureServe(routeId, host, method string, code int, start time.Time) {
	a.prometheus.MeasureServe(routeId, host, method, code, start)
	a.codaHale.MeasureServe(routeId, host, method, code, start)
}
func (a *All) IncRoutingFailures() {
	a.prometheus.IncRoutingFailures()
	a.codaHale.IncRoutingFailures()
}
func (a *All) IncErrorsBackend(routeId string) {
	a.prometheus.IncErrorsBackend(routeId)
	a.codaHale.IncErrorsBackend(routeId)
}
func (a *All) MeasureBackend5xx(t time.Time) {
	a.prometheus.MeasureBackend5xx(t)
	a.codaHale.MeasureBackend5xx(t)

}
func (a *All) IncErrorsStreaming(routeId string) {
	a.prometheus.IncErrorsStreaming(routeId)
	a.codaHale.IncErrorsStreaming(routeId)

}
func (a *All) RegisterHandler(path string, handler *http.ServeMux) {
	a.prometheusHandler = a.prometheus.getHandler()
	a.codaHaleHandler = a.codaHale.getHandler(path)
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
