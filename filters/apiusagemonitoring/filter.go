package apiusagemonitoring

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/zalando/skipper/filters"
	"net/http"
	"strings"
	"time"
)

const (
	metricCountAll   = "http_count"
	metricCount100s  = "http1xx_count"
	metricCount200s  = "http2xx_count"
	metricCount300s  = "http3xx_count"
	metricCount400s  = "http4xx_count"
	metricCount500s  = "http5xx_count"
	metricLatency    = "latency"
	metricLatencySum = "latency_sum"
)

var (
	metricCountPerClass = [5]string{
		metricCount100s,
		metricCount200s,
		metricCount300s,
		metricCount400s,
		metricCount500s,
	}
)

const (
	stateBagKeyPrefix = "filter." + Name + "."
	stateBagKeyBegin  = stateBagKeyPrefix + "begin"
)

const (
	authorizationHeaderName   = "Authorization"
	authorizationHeaderPrefix = "Bearer "
)

// apiUsageMonitoringFilter implements filters.Filter interface and is the structure
// created for every route invocation of the `apiUsageMonitoring` filter.
type apiUsageMonitoringFilter struct {
	Spec  *apiUsageMonitoringSpec
	Paths []*pathInfo
}

func (f *apiUsageMonitoringFilter) Request(c filters.FilterContext) {
	// Gathering information from the initial request for further metrics calculation
	now := time.Now()
	c.StateBag()[stateBagKeyBegin] = now
}

func (f *apiUsageMonitoringFilter) Response(c filters.FilterContext) {
	request, response, metrics := c.Request(), c.Response(), c.Metrics()
	begin, beginPresent := c.StateBag()[stateBagKeyBegin].(time.Time)
	path, metricsName := f.resolvePath(request)

	// METRIC: Count
	metrics.IncCounter(metricsName.CountAll)

	// METRIC: Response Status Range Count
	classMetricsIndex := (response.StatusCode / 100) - 1
	if classMetricsIndex < 0 || classMetricsIndex >= 5 {
		log.Errorf(
			"Response HTTP Status Code %d is invalid. Response status code metric not tracked for this call.",
			response.StatusCode)
	} else {
		metrics.IncCounter(metricsName.CountPerStatusCodeRange[classMetricsIndex])
	}

	// METRIC: Latency
	if beginPresent {
		metrics.MeasureSince(metricsName.Latency, begin)
	}

	// Client Based Metrics
	if path.ClientTracking != nil {
		cmPre := metricsName.ClientPrefix + f.determineClientMetricPart(c, path) + "."

		// METRIC: Count for client
		metrics.IncCounter(cmPre + metricCountAll)

		// METRIC: Response Status Range Count for client
		metrics.IncCounter(cmPre + metricCountPerClass[classMetricsIndex])

		// METRIC: Latency Sum (in decimal seconds)
		if beginPresent {
			latency := time.Since(begin).Seconds()
			metrics.IncFloatCounterBy(cmPre+metricLatencySum, latency)
		}

		log.Debugf("Pushed client metrics with prefix `%s`", cmPre)
	}

	log.Debugf("Pushed endpoint metrics with prefix `%s`", metricsName.EndpointPrefix)
}

// determineClientMetricPart generates the proper <Realm>.<Client ID> part of the
// client metrics name.
func (f *apiUsageMonitoringFilter) determineClientMetricPart(c filters.FilterContext, path *pathInfo) string {
	// if no JWT: track <unknown>.<unknown>
	jwt := parseJwtBody(c.Request())
	if jwt == nil {
		return unknownElementPlaceholder + "." + unknownElementPlaceholder
	}

	// if no realm in JWT: track <unknown>.<unknown>
	realm, ok := jwt[f.Spec.realmKey].(string)
	if !ok {
		return unknownElementPlaceholder + "." + unknownElementPlaceholder
	}

	// if not matcher: track realm.<unknown>
	if path.ClientTracking.ClientTrackingMatcher == nil {
		return realm + "." + unknownElementPlaceholder
	}

	// if no client ID in JWT: track realm.<unknown>
	var clientId string
	for _, k := range f.Spec.clientIdKey {
		if clientId, ok = jwt[k].(string); ok {
			break
		}
	}
	if !ok {
		return realm + "." + unknownElementPlaceholder
	}

	// if no realm.client does not match, track realm.<unknown>
	realmAndClient := realm + "." + clientId
	if !path.ClientTracking.ClientTrackingMatcher.MatchString(realmAndClient) {
		return realm + "." + unknownElementPlaceholder
	}

	// all matched: track realm.client_id
	return realmAndClient
}

// String returns a JSON representation of the filter prefixed by its type.
func (f *apiUsageMonitoringFilter) String() string {
	var js string
	if jsBytes, err := json.Marshal(f); err == nil {
		js = string(jsBytes)
	} else {
		js = fmt.Sprintf("<%v>", err)
	}
	return fmt.Sprintf("%T %s", f, js)
}

// resolvePath returns the structure with names of the metrics for this specific context.
// If it is not already cached, it is generated and cached to speed up next calls.
func (f *apiUsageMonitoringFilter) resolvePath(req *http.Request) (*pathInfo, *metricNames) {

	// Match the path to a known template
	var path *pathInfo = nil
	for _, p := range f.Paths {
		if p.Matcher.MatchString(req.URL.Path) {
			path = p
			break
		}
	}
	if path == nil {
		path = f.Spec.unknownPath
	}

	// Get the cached prefixes for this path and verb
	method := req.Method
	methodIndex, ok := methodToIndex[method]
	if !ok {
		methodIndex = MethodIndexUnknown
		method = unknownElementPlaceholder
	}

	prefixes := path.metricPrefixesPerMethod[methodIndex]
	if prefixes != nil {
		return path, prefixes
	}

	// Prefixes were not cached for this path and method. Generate and cache.
	prefixCommon := path.ApplicationId + "." + path.ApiId + "."
	endpointPrefix := prefixCommon + method + "." + path.PathTemplate + ".*.*."
	clientPrefix := prefixCommon + "*.*."
	prefixes = &metricNames{
		EndpointPrefix: endpointPrefix,
		ClientPrefix:   clientPrefix,
		CountAll:       endpointPrefix + metricCountAll,
		CountPerStatusCodeRange: [5]string{
			endpointPrefix + metricCount100s,
			endpointPrefix + metricCount200s,
			endpointPrefix + metricCount300s,
			endpointPrefix + metricCount400s,
			endpointPrefix + metricCount500s,
		},
		Latency: endpointPrefix + metricLatency,
	}
	path.metricPrefixesPerMethod[methodIndex] = prefixes
	return path, prefixes
}

// parseJwtBody parses the JWT token from a HTTP request.
// It returns `nil` if it was not possible to parse the JWT body.
func parseJwtBody(req *http.Request) map[string]interface{} {
	ahead := req.Header.Get(authorizationHeaderName)
	if !strings.HasPrefix(ahead, authorizationHeaderPrefix) {
		return nil
	}

	// split the header into the 3 JWT parts
	fields := strings.Split(ahead, ".")
	if len(fields) != 3 {
		return nil
	}

	// base64-decode the JWT body part
	bodyJson, err := base64.RawURLEncoding.DecodeString(fields[1])
	if err != nil {
		return nil
	}

	// un-marshall the JWT body from JSON
	var bodyObject map[string]interface{}
	err = json.Unmarshal(bodyJson, &bodyObject)
	if err != nil {
		return nil
	}

	return bodyObject
}
