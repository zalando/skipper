package innkeeper

import (
	"errors"
	"github.com/zalando/skipper/eskip"
	"log"
	"math"
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

func checkArgs(args ...interface{}) error {
	for _, a := range args {
		if f, ok := a.(float64); ok && (f != math.Trunc(f) || f != float64(int32(f))) {
			return errors.New("only 32 bit integers are supported by innkeeper")
		}
	}

	return nil
}

func convertArgs(args []interface{}) ([]interface{}, error) {
	cargs := make([]interface{}, len(args))
	for i, a := range args {
		if err := checkArgs(a); err == nil {
			cargs[i] = a
		} else {
			return nil, err
		}
	}

	return cargs, nil
}

func convertEskipPredicates(r *eskip.Route) ([]customPredicate, error) {
	var ps []customPredicate
	for _, p := range r.Predicates {
		args, err := convertArgs(p.Args)
		if err != nil {
			return nil, err
		}

		ps = append(ps, customPredicate{
			Name: p.Name,
			Args: args})
	}

	return ps, nil
}

func convertFil(r *eskip.Route) (filters []filter, err error) {
	filters = []filter{}
	for _, f := range r.Filters {

		var args = []interface{}{}
		if f.Args != nil {
			args = f.Args
		}

		args, err = convertArgs(args)
		if err != nil {
			return
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

func convertEskipToInnkeeper(routes []*eskip.Route) ([]*routeData, error) {
	var data []*routeData

	for _, r := range routes {

		id := eskip.GenerateIfNeeded(r.Id)
		host := convertHost(r)
		method := convertMethod(r)
		pathMatch := convertPathMatcher(r)
		headerMatchers := convertHeaderMatchers(r)

		predicates, err := convertEskipPredicates(r)
		if err != nil {
			return nil, err
		}

		filters, err := convertFil(r)
		if err != nil {
			return nil, err
		}

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

		d := &routeData{
			Name:  id,
			Route: *ro}

		data = append(data, d)
	}

	return data, nil
}
