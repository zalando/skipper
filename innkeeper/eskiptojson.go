package innkeeper

import (
	"github.com/zalando/skipper/eskip"
	"log"
)

type matchType string

const (
	matchStrict = matchType("STRICT")
	matchRegex  = matchType("REGEX")
)

type (
	pathMatcher struct {
		Typ   matchType `json:"type,omitempty"`
		Match string    `json:"match,omitempty"`
	}

	headerMatcher struct {
		Typ   matchType `json:"type,omitempty"`
		Name  string    `json:"name,omitempty"`
		Value string    `json:"value,omitempty"`
	}

	customPredicate struct {
		Name string        `json:"name,omitempty"`
		Args []interface{} `json:"args"`
	}

	filter struct {
		Name string        `json:"name,omitempty"`
		Args []interface{} `json:"args"`
	}

	matcher struct {
		HostMatcher    string          `json:"host_matcher,omitempty"`
		PathMatcher    *pathMatcher    `json:"path_matcher,omitempty"`
		MethodMatcher  string          `json:"method_matcher,omitempty"`
		HeaderMatchers []headerMatcher `json:"header_matchers"`
	}

	routeDef struct {
		Matcher    matcher           `json:"matcher,omitempty"`
		Predicates []customPredicate `json:"predicates"`
		Filters    []filter          `json:"filters"`
		Endpoint   string            `json:"endpoint,omitempty"`
	}

	jsonRoute struct {
		Id         int64    `json:"id,omitempty"`
		Name       string   `json:"name,omitempty"`
		ActivateAt string   `json:"activate_at,omitempty"`
		CreatedAt  string   `json:"created_at,omitempty"`
		DeletedAt  string   `json:"deleted_at,omitempty"`
		Route      routeDef `json:"route"`
	}
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
		if len(r.PathRegexps) > 1 {
			log.Println("Warn: We should only have one path regexp")
		}
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
		if len(r.HostRegexps) > 1 {
			log.Println("Warn: We should only have one host regexp")
		}
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

func convertEskipPredicates(r *eskip.Route) []customPredicate {
	var ps []customPredicate
	for _, p := range r.Predicates {
		ps = append(ps, customPredicate{
			Name: p.Name,
			Args: p.Args})
	}

	return ps
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

func convertEskipToInnkeeper(routes []*eskip.Route) (data []*jsonRoute) {
	for _, r := range routes {

		id := eskip.GenerateIfNeeded(r.Id)
		host := convertHost(r)
		method := convertMethod(r)
		pathMatch := convertPathMatcher(r)
		headerMatchers := convertHeaderMatchers(r)
		predicates := convertEskipPredicates(r)
		filters := convertFil(r)
		endpoint := convertEndpoint(r)

		match := &matcher{
			HostMatcher:    host,
			PathMatcher:    pathMatch,
			MethodMatcher:  method,
			HeaderMatchers: headerMatchers}

		ro := &routeDef{
			Matcher:    *match,
			Predicates: predicates,
			Filters:    filters,
			Endpoint:   endpoint}

		d := &jsonRoute{
			Name:  id,
			Route: *ro}

		data = append(data, d)
	}

	return
}
