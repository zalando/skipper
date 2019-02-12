package apiusagemonitoring

import (
	"net/http"
	"regexp"
)

// pathInfo contains the tracking information for a specific path.
type pathInfo struct {
	ApplicationId  string
	ApiId          string
	PathTemplate   string
	PathLabel      string
	Matcher        *regexp.Regexp
	ClientTracking *clientTrackingInfo
	CommonPrefix   string
	ClientPrefix   string

	metricPrefixesPerMethod [methodIndexLength]*endpointMetricNames
	metricPrefixedPerClient map[string]*clientMetricNames
}

func newPathInfo(applicationId, apiId, pathTemplate string, pathLabel string, clientTracking *clientTrackingInfo) *pathInfo {
	commonPrefix := applicationId + "." + apiId + "."
	var metricPrefixedPerClient map[string]*clientMetricNames
	if clientTracking != nil {
		metricPrefixedPerClient = make(map[string]*clientMetricNames)
	}
	return &pathInfo{
		ApplicationId:           applicationId,
		ApiId:                   apiId,
		PathTemplate:            pathTemplate,
		PathLabel:               pathLabel,
		metricPrefixedPerClient: metricPrefixedPerClient,
		ClientTracking:          clientTracking,
		CommonPrefix:            commonPrefix,
		ClientPrefix:            commonPrefix + "*.*.",
	}
}

// pathInfoByRegExRev allows sort.Sort to reorder a slice of `pathInfo` in
// reverse alphabetical order of their matcher (Regular Expression). That way,
// the more complex RegEx will end up first in the slice.
type pathInfoByRegExRev []*pathInfo

func (s pathInfoByRegExRev) Len() int           { return len(s) }
func (s pathInfoByRegExRev) Less(i, j int) bool { return s[i].Matcher.String() > s[j].Matcher.String() }
func (s pathInfoByRegExRev) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type endpointMetricNames struct {
	endpointPrefix          string
	countAll                string
	countPerStatusCodeRange [6]string
	latency                 string
}

type clientMetricNames struct {
	countAll                string
	countPerStatusCodeRange [6]string
	latencySum              string
}

const (
	methodIndexGet     = iota // GET
	methodIndexHead           // HEAD
	methodIndexPost           // POST
	methodIndexPut            // PUT
	methodIndexPatch          // PATCH
	methodIndexDelete         // DELETE
	methodIndexConnect        // CONNECT
	methodIndexOptions        // OPTIONS
	methodIndexTrace          // TRACE

	methodIndexUnknown // Value when the HTTP Method is not in the known list
	methodIndexLength  // Gives the constant size of the `metricPrefixesPerMethod` array.
)

var (
	methodToIndex = map[string]int{
		http.MethodGet:     methodIndexGet,
		http.MethodHead:    methodIndexHead,
		http.MethodPost:    methodIndexPost,
		http.MethodPut:     methodIndexPut,
		http.MethodPatch:   methodIndexPatch,
		http.MethodDelete:  methodIndexDelete,
		http.MethodConnect: methodIndexConnect,
		http.MethodOptions: methodIndexOptions,
		http.MethodTrace:   methodIndexTrace,
	}
)

type clientTrackingInfo struct {
	ClientTrackingMatcher *regexp.Regexp
	RealmsToTrack         []string
}
