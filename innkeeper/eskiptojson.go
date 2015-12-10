package innkeeper

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routeid"
)

func convertPathMatcher(r *eskip.Route) *pathMatcher {
	var (
		pathMatch     string
		pathMatchType matchType
	)

	if r.Path != "" {
		pathMatch = r.Path
		pathMatchType = matchStrict
	} else if len(r.PathRegexps) > 0 {
		// TODO we should only have one path regexp
		pathMatch = r.PathRegexps[0]
		pathMatchType = matchRegex
	} else {
		return nil
	}

	return &pathMatcher{
		Match: pathMatch,
		Typ:   pathMatchType}
}

func convertMethod(r *eskip.Route) string {
	return r.Method
}

func convertHost(r *eskip.Route) (host string) {
	if len(r.HostRegexps) > 0 {
		// we take the first one
		// TODO HostRegexps should not be an array
		host = r.HostRegexps[0]
	}
	return
}

func appendHeader(headers *map[string]string, hm []headerMatcher, ty matchType) (headerMatchers []headerMatcher) {
	headerMatchers = hm
	for k, v := range *headers {
		headerMatchers = append(headerMatchers, headerMatcher{
			Name:  k,
			Value: v,
			Typ:   ty})
	}
	return
}

func convertHeaderMatchers(r *eskip.Route) (headerMatchers []headerMatcher) {
	headerMatchers = []headerMatcher{}
	headerMatchers = appendHeader(&r.Headers, headerMatchers, matchStrict)
	// TODO there should only be one map of header regexp
	//headerMatchers = appendHeader(&r.HeaderRegexps[0], headerMatchers, matchRegexp)

	for k, l := range r.HeaderRegexps {
		for _, v := range l {
			headerMatchers = append(headerMatchers, headerMatcher{
				Name:  k,
				Value: v,
				Typ:   matchRegex})
		}
	}
	return
}

func convertFil(r *eskip.Route) (filters []filter) {
	filters = []filter{}
	for _, f := range r.Filters {

		var args = []interface{}{}
		if f.Args != nil {
			args = f.Args
		}

		filters = append(filters, filter{
			Name: f.Name,
			Args: args})
	}
	return
}

func convertEndpoint(r *eskip.Route) (endpoint string) {
	if r.Shunt == false && r.Backend != "" {
		endpoint = r.Backend
	}
	return
}

func convertEskipToInnkeeper(routes []*eskip.Route) (data []*routeData) {

	for _, r := range routes {

		id := routeid.GenerateIfNeeded(r.Id)
		host := convertHost(r)
		method := convertMethod(r)
		pathMatch := convertPathMatcher(r)
		headerMatchers := convertHeaderMatchers(r)
		filters := convertFil(r)
		endpoint := convertEndpoint(r)

		match := &matcher{
			HostMatcher:    host,
			PathMatcher:    pathMatch,
			MethodMatcher:  method,
			HeaderMatchers: headerMatchers}

		ro := &routeDef{
			Matcher:  *match,
			Filters:  filters,
			Endpoint: endpoint}

		d := &routeData{
			Name:  id,
			Route: *ro}

		data = append(data, d)
	}

	return
}
