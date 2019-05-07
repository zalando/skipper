package apiusagemonitoring

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zalando/skipper/filters"
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
	Spec        *apiUsageMonitoringSpec
	Paths       []*pathInfo
	UnknownPath *pathInfo
}

func (f *apiUsageMonitoringFilter) Request(c filters.FilterContext) {
	c.StateBag()[stateBagKeyBegin] = time.Now()
}

func (f *apiUsageMonitoringFilter) Response(c filters.FilterContext) {
	request, response, metrics := c.Request(), c.Response(), c.Metrics()
	begin, beginPresent := c.StateBag()[stateBagKeyBegin].(time.Time)
	path := f.resolveMatchedPath(request.URL)

	classMetricsIndex := response.StatusCode / 100
	if classMetricsIndex < 1 || classMetricsIndex > 5 {
		log.Errorf(
			"Response HTTP Status Code %d is invalid. Response status code metric will be %q.",
			response.StatusCode, metricCountUnknownClass)
		classMetricsIndex = 0 // unknown classes are tracked, not ignored
	}

	// Endpoint metrics
	endpointMetricsNames := getEndpointMetricsNames(request, path)
	metrics.IncCounter(endpointMetricsNames.countAll)
	metrics.IncCounter(endpointMetricsNames.countPerStatusCodeRange[classMetricsIndex])
	if beginPresent {
		metrics.MeasureSince(endpointMetricsNames.latency, begin)
	}
	log.Debugf("Pushed endpoint metrics with prefix `%s`", endpointMetricsNames.endpointPrefix)

	// Client metrics
	if path.ClientTracking != nil {
		realmClientKey := f.getRealmClientKey(request, path)
		clientMetricsNames := getClientMetricsNames(realmClientKey, path)
		metrics.IncCounter(clientMetricsNames.countAll)
		metrics.IncCounter(clientMetricsNames.countPerStatusCodeRange[classMetricsIndex])
		if beginPresent {
			latency := time.Since(begin).Seconds()
			metrics.IncFloatCounterBy(clientMetricsNames.latencySum, latency)
		}
		log.Debugf("Pushed client metrics with prefix `%s%s.`", path.ClientPrefix, realmClientKey)
	}
}

func getClientMetricsNames(realmClientKey string, path *pathInfo) *clientMetricNames {
	if value, ok := path.metricPrefixedPerClient.Load(realmClientKey); ok {
		if prefixes, ok := value.(clientMetricNames); ok {
			return &prefixes
		}
	}

	clientPrefixForThisClient := path.ClientPrefix + realmClientKey + "."
	prefixes := &clientMetricNames{
		countAll: clientPrefixForThisClient + metricCountAll,
		countPerStatusCodeRange: [6]string{
			clientPrefixForThisClient + metricCountUnknownClass,
			clientPrefixForThisClient + metricCount100s,
			clientPrefixForThisClient + metricCount200s,
			clientPrefixForThisClient + metricCount300s,
			clientPrefixForThisClient + metricCount400s,
			clientPrefixForThisClient + metricCount500s,
		},
		latencySum: clientPrefixForThisClient + metricLatencySum,
	}
	path.metricPrefixedPerClient.Store(realmClientKey, prefixes)
	return prefixes
}

const unknownUnknown = unknownPlaceholder + "." + unknownPlaceholder

// getRealmClientKey generates the proper <realm>.<client> part of the client metrics name.
func (f *apiUsageMonitoringFilter) getRealmClientKey(r *http.Request, path *pathInfo) string {
	// no JWT ==> {unknown}.{unknown}
	jwt := parseJwtBody(r)
	if jwt == nil {
		return unknownUnknown
	}

	// no realm in JWT ==> {unknown}.{unknown}
	realm, ok := jwt.getOneOfString(f.Spec.realmKeys)
	if !ok {
		return unknownUnknown
	}

	// realm is not one of the realmsTrackingPattern to be tracked ==> realm.{all}
	if !path.ClientTracking.RealmsTrackingMatcher.MatchString(realm) {
		return realm + ".{all}"
	}

	// no client in JWT ==> realm.{unknown}
	client, ok := jwt.getOneOfString(f.Spec.clientKeys)
	if !ok {
		return realm + "." + unknownPlaceholder
	}

	// if client does not match ==> realm.{no-match}
	matcher := path.ClientTracking.ClientTrackingMatcher
	if matcher == nil || !matcher.MatchString(client) {
		return realm + "." + noMatchPlaceholder
	}

	// all matched ==> realm.client
	return realm + "." + client
}

// resolveMatchedPath tries to match the request's path with one of the configured path template.
func (f *apiUsageMonitoringFilter) resolveMatchedPath(url *url.URL) *pathInfo {
	if url != nil {
		for _, p := range f.Paths {
			if p.Matcher.MatchString(url.Path) {
				return p
			}
		}
	}
	return f.UnknownPath
}

// getEndpointMetricsNames returns the structure with names of the metrics for this specific context.
// It tries first from the path's cache. If it is not already cached, it is generated and
// caches it to speed up next calls.
func getEndpointMetricsNames(req *http.Request, path *pathInfo) *endpointMetricNames {
	method := req.Method
	methodIndex, ok := methodToIndex[method]
	if !ok {
		methodIndex = methodIndexUnknown
		method = unknownPlaceholder
	}

	if p := path.metricPrefixesPerMethod[methodIndex]; p != nil {
		return p
	}
	return createAndCacheMetricsNames(path, method, methodIndex)
}

// createAndCacheMetricsNames generates metrics names and cache them.
func createAndCacheMetricsNames(path *pathInfo, method string, methodIndex int) *endpointMetricNames {
	endpointPrefix := path.CommonPrefix + method + "." + path.PathLabel + ".*.*."
	prefixes := &endpointMetricNames{
		endpointPrefix: endpointPrefix,
		countAll:       endpointPrefix + metricCountAll,
		countPerStatusCodeRange: [6]string{
			endpointPrefix + metricCountUnknownClass,
			endpointPrefix + metricCount100s,
			endpointPrefix + metricCount200s,
			endpointPrefix + metricCount300s,
			endpointPrefix + metricCount400s,
			endpointPrefix + metricCount500s,
		},
		latency: endpointPrefix + metricLatency,
	}
	path.metricPrefixesPerMethod[methodIndex] = prefixes
	return prefixes
}

// parseJwtBody parses the JWT token from a HTTP request.
// It returns `nil` if it was not possible to parse the JWT body.
func parseJwtBody(req *http.Request) jwtTokenPayload {
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
	bodyJSON, err := base64.RawURLEncoding.DecodeString(fields[1])
	if err != nil {
		return nil
	}

	// un-marshall the JWT body from JSON
	var bodyObject map[string]interface{}
	err = json.Unmarshal(bodyJSON, &bodyObject)
	if err != nil {
		return nil
	}

	return bodyObject
}

type jwtTokenPayload map[string]interface{}

func (j jwtTokenPayload) getOneOfString(properties []string) (value string, ok bool) {
	var rawValue interface{}
	for _, p := range properties {
		if rawValue, ok = j[p]; ok {
			value = fmt.Sprint(rawValue)
			return
		}
	}
	return
}
