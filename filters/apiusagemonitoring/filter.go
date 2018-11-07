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
	metricCountAll          = "http_count"
	metricCountUnknownClass = "httpxxx_count"
	metricCount100s         = "http1xx_count"
	metricCount200s         = "http2xx_count"
	metricCount300s         = "http3xx_count"
	metricCount400s         = "http4xx_count"
	metricCount500s         = "http5xx_count"
	metricLatency           = "latency"
	metricLatencySum        = "latency_sum"
)

var (
	metricCountPerClass = [6]string{
		metricCountUnknownClass,
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
	c.StateBag()[stateBagKeyBegin] = time.Now()
}

func (f *apiUsageMonitoringFilter) Response(c filters.FilterContext) {
	request, response, metrics := c.Request(), c.Response(), c.Metrics()
	begin, beginPresent := c.StateBag()[stateBagKeyBegin].(time.Time)
	path := f.resolvePath(request)
	metricsName := getMetricsNames(request, path)

	classMetricsIndex := response.StatusCode / 100
	if classMetricsIndex < 1 || classMetricsIndex > 5 {
		log.Errorf(
			"Response HTTP Status Code %d is invalid. Response status code metric will be %q.",
			response.StatusCode, metricCountUnknownClass)
		classMetricsIndex = 0
	}

	// METRIC: Count
	metrics.IncCounter(metricsName.CountAll)

	// METRIC: Response Status Range Count
	metrics.IncCounter(metricsName.CountPerStatusCodeRange[classMetricsIndex])

	// METRIC: Latency
	if beginPresent {
		metrics.MeasureSince(metricsName.Latency, begin)
	}

	// Client Based Metrics
	if path.ClientTracking != nil {
		cmPre := path.ClientPrefix + f.getRealmClientKey(request, path) + "."

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

// getRealmClientKey generates the proper <Realm>.<Client ID> part of the
// client metrics name.
func (f *apiUsageMonitoringFilter) getRealmClientKey(r *http.Request, path *pathInfo) string {
	// if no JWT: track <unknown>.<unknown>
	jwt := parseJwtBody(r)
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

// resolvePath tries to match the request's path with one of the configured path template.
func (f *apiUsageMonitoringFilter) resolvePath(req *http.Request) *pathInfo {
	for _, p := range f.Paths {
		if p.Matcher.MatchString(req.URL.Path) {
			return p
		}
	}
	return f.Spec.unknownPath
}

// getMetricsNames returns the structure with names of the metrics for this specific context.
// It tries first from the path's cache. If it is not already cached, it is generated and
// caches it to speed up next calls.
func getMetricsNames(req *http.Request, path *pathInfo) *metricNames {
	method := req.Method
	methodIndex, ok := methodToIndex[method]
	if !ok {
		methodIndex = MethodIndexUnknown
		method = unknownElementPlaceholder
	}

	prefixes := path.metricPrefixesPerMethod[methodIndex]
	if prefixes == nil {
		return createAndCacheMetricsNames(path, method, methodIndex)
	} else {
		return prefixes
	}
}

// createAndCacheMetricsNames generates metrics names and cache them.
func createAndCacheMetricsNames(path *pathInfo, method string, methodIndex int) *metricNames {
	endpointPrefix := path.CommonPrefix + method + "." + path.PathTemplate + ".*.*."
	prefixes := &metricNames{
		EndpointPrefix: endpointPrefix,
		CountAll:       endpointPrefix + metricCountAll,
		CountPerStatusCodeRange: [6]string{
			endpointPrefix + metricCountUnknownClass,
			endpointPrefix + metricCount100s,
			endpointPrefix + metricCount200s,
			endpointPrefix + metricCount300s,
			endpointPrefix + metricCount400s,
			endpointPrefix + metricCount500s,
		},
		Latency: endpointPrefix + metricLatency,
	}
	path.metricPrefixesPerMethod[methodIndex] = prefixes
	return prefixes
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
