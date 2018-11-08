package apiusagemonitoring

import (
	"encoding/json"
	"net/http"
	"regexp"
)

// pathInfo contains the tracking information for a specific path.
type pathInfo struct {
	ApplicationId           string
	ApiId                   string
	PathTemplate            string
	Matcher                 *regexp.Regexp
	metricPrefixesPerMethod [MethodIndexLength]*endpointMetricNames
	metricPrefixedPerClient map[string]*clientMetricNames
	ClientTracking          *clientTrackingInfo
	CommonPrefix            string
	ClientPrefix            string
}

func newPathInfo(applicationId, apiId, pathTemplate string, clientTracking *clientTrackingInfo) *pathInfo {
	commonPrefix := applicationId + "." + apiId + "."
	var metricPrefixedPerClient map[string]*clientMetricNames
	if clientTracking != nil {
		metricPrefixedPerClient = make(map[string]*clientMetricNames)
	}
	return &pathInfo{
		ApplicationId:           applicationId,
		ApiId:                   apiId,
		PathTemplate:            pathTemplate,
		metricPrefixedPerClient: metricPrefixedPerClient,
		ClientTracking:          clientTracking,
		CommonPrefix:            commonPrefix,
		ClientPrefix:            commonPrefix + "*.*.",
	}
}

func (p *pathInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationId string
		ApiId         string
		PathTemplate  string
		Matcher       string
	}{
		ApplicationId: p.ApplicationId,
		ApiId:         p.ApiId,
		PathTemplate:  p.PathTemplate,
		Matcher:       p.Matcher.String(),
	})
}

// pathInfoByRegExRev allows sort.Sort to reorder a slice of `pathInfo` in
// reverse alphabetical order of their matcher (Regular Expression). That way,
// the more complex RegEx will end up first in the slice.
type pathInfoByRegExRev []*pathInfo

func (s pathInfoByRegExRev) Len() int           { return len(s) }
func (s pathInfoByRegExRev) Less(i, j int) bool { return s[i].Matcher.String() > s[j].Matcher.String() }
func (s pathInfoByRegExRev) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type endpointMetricNames struct {
	EndpointPrefix          string
	CountAll                string
	CountPerStatusCodeRange [6]string
	Latency                 string
}

type clientMetricNames struct {
	CountAll                string
	CountPerStatusCodeRange [6]string
	LatencySum              string
}

const (
	MethodIndexGet     = iota // GET
	MethodIndexHead           // HEAD
	MethodIndexPost           // POST
	MethodIndexPut            // PUT
	MethodIndexPatch          // PATCH
	MethodIndexDelete         // DELETE
	MethodIndexConnect        // CONNECT
	MethodIndexOptions        // OPTIONS
	MethodIndexTrace          // TRACE

	MethodIndexUnknown // Value when the HTTP Method is not in the known list
	MethodIndexLength  // Gives the constant size of the `metricPrefixesPerMethod` array.
)

var (
	methodToIndex = map[string]int{
		http.MethodGet:     MethodIndexGet,
		http.MethodHead:    MethodIndexHead,
		http.MethodPost:    MethodIndexPost,
		http.MethodPut:     MethodIndexPut,
		http.MethodPatch:   MethodIndexPatch,
		http.MethodDelete:  MethodIndexDelete,
		http.MethodConnect: MethodIndexConnect,
		http.MethodOptions: MethodIndexOptions,
		http.MethodTrace:   MethodIndexTrace,
	}
)

type clientTrackingInfo struct {
	ClientTrackingMatcher *regexp.Regexp
}
