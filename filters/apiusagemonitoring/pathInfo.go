package apiusagemonitoring

import (
	"encoding/json"
	"regexp"
)

var (
	unknownPath = &pathInfo{
		ApplicationId:           unknownElementPlaceholder,
		ApiId:                   unknownElementPlaceholder,
		PathTemplate:            unknownElementPlaceholder,
		metricPrefixesPerMethod: make(map[string]*specificMetricsName),
	}
)

type pathInfo struct {
	ApplicationId           string
	ApiId                   string
	PathTemplate            string
	Matcher                 *regexp.Regexp
	metricPrefixesPerMethod map[string]*specificMetricsName
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

type specificMetricsName struct {
	GlobalPrefix            string
	CountAll                string
	CountPerStatusCodeRange [5]string
	Latency                 string
}
