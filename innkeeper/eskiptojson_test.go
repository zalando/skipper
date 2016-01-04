package innkeeper

import (
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"testing"
)

func TestConvertPathMatcherStrict(t *testing.T) {

	route := &eskip.Route{
		Path: "/hello"}

	matcher := convertPathMatcher(route)
	assert.Equal(t, "/hello", matcher.Match)
	assert.Equal(t, matchStrict, matcher.Typ)
}

func TestConvertPathMatcherRegex(t *testing.T) {

	route := &eskip.Route{
		PathRegexps: []string{"/hello*"}}

	matcher := convertPathMatcher(route)
	assert.Equal(t, "/hello*", matcher.Match)
	assert.Equal(t, matchRegex, matcher.Typ)
}

func TestConvertPathMatcherEmpty(t *testing.T) {

	route := &eskip.Route{}

	matcher := convertPathMatcher(route)
	var expectedMatcher *pathMatcher = nil
	assert.Equal(t, expectedMatcher, matcher)
}

func TestConvertMethod(t *testing.T) {
	route := &eskip.Route{
		Method: "GET"}

	method := convertMethod(route)

	assert.Equal(t, "GET", method)
}

func TestConvertMethodEmpty(t *testing.T) {
	route := &eskip.Route{}

	method := convertMethod(route)

	assert.Equal(t, "", method)
}

func TestConvertHost(t *testing.T) {
	route := &eskip.Route{HostRegexps: []string{"www.regex.com"}}

	host := convertHost(route)

	assert.Equal(t, "www.regex.com", host)
}

func TestConvertHostEmpty(t *testing.T) {
	route := &eskip.Route{}

	host := convertHost(route)

	assert.Equal(t, "", host)
}

func TestConvertHeaderMatchers(t *testing.T) {
	route := &eskip.Route{
		HeaderRegexps: map[string][]string{"header": []string{"first"}},
		Headers:       map[string]string{"header1": "second"}}

	headerMatchers := convertHeaderMatchers(route)
	assert.Equal(t, 2, len(headerMatchers))
	assert.Equal(t, "header1", headerMatchers[0].Name)
	assert.Equal(t, "second", headerMatchers[0].Value)
	assert.Equal(t, matchStrict, headerMatchers[0].Typ)

	assert.Equal(t, "header", headerMatchers[1].Name)
	assert.Equal(t, "first", headerMatchers[1].Value)
	assert.Equal(t, matchRegex, headerMatchers[1].Typ)
}

func TestConvertHeaderMatchersEmpty(t *testing.T) {
	route := &eskip.Route{}

	headerMatchers := convertHeaderMatchers(route)

	assert.Equal(t, []headerMatcher{}, headerMatchers)
}

func TestConvertFilWithArgs(t *testing.T) {
	route := &eskip.Route{Filters: []*eskip.Filter{
		&eskip.Filter{Name: "filter1", Args: []interface{}{"Hello", 1}},
		&eskip.Filter{Name: "filter2", Args: []interface{}{2, "Hello1", "World"}},
	}}

	filters := convertFil(route)
	assert.Equal(t, 2, len(filters))
	assert.Equal(t, "filter1", filters[0].Name)
	assert.Equal(t, 2, len(filters[0].Args))
	assert.Equal(t, "Hello", filters[0].Args[0])
	assert.Equal(t, 1, filters[0].Args[1])
	assert.Equal(t, "filter2", filters[1].Name)
	assert.Equal(t, 3, len(filters[1].Args))
	assert.Equal(t, 2, filters[1].Args[0])
	assert.Equal(t, "Hello1", filters[1].Args[1])
	assert.Equal(t, "World", filters[1].Args[2])
}

func TestConvertFilWithoutArgs(t *testing.T) {
	route := &eskip.Route{Filters: []*eskip.Filter{
		&eskip.Filter{Name: "filter1", Args: []interface{}{}},
		&eskip.Filter{Name: "filter2"},
	}}

	filters := convertFil(route)
	assert.Equal(t, 2, len(filters))
	assert.Equal(t, "filter1", filters[0].Name)
	assert.Equal(t, 0, len(filters[0].Args))
	assert.Equal(t, "filter2", filters[1].Name)
	assert.Equal(t, 0, len(filters[1].Args))
}

func TestConvertFilEmpty(t *testing.T) {
	route := &eskip.Route{}

	filters := convertFil(route)

	assert.Equal(t, []filter{}, filters)
}

func TestConvertEndpoint(t *testing.T) {
	route := &eskip.Route{Backend: "www.example.com"}

	endpoint := convertEndpoint(route)

	assert.Equal(t, "www.example.com", endpoint)
}

func TestConvertEndpointEmpty(t *testing.T) {
	route := &eskip.Route{}

	endpoint := convertEndpoint(route)

	assert.Equal(t, "", endpoint)
}

func TestConvertEskipToInnkeeper(t *testing.T) {

	route := []*eskip.Route{{
		Id:            "theid",
		HostRegexps:   []string{"www.matcher.com"},
		Method:        "GET",
		PathRegexps:   []string{"/hello*"},
		HeaderRegexps: map[string][]string{"header": []string{"first"}},
		Headers:       map[string]string{"header1": "second"},
		Filters: []*eskip.Filter{
			&eskip.Filter{Name: "filter1", Args: []interface{}{"Hello", 1}},
			&eskip.Filter{Name: "filter2", Args: []interface{}{2, "Hello1", "World"}}},
		Backend: "www.backend.com"}}

	routes := convertEskipToInnkeeper(route)

	assert.Equal(t, 1, len(routes))
	assert.Equal(t, "theid", routes[0].Name)
	assert.Equal(t, 2, len(routes[0].Route.Matcher.HeaderMatchers))
	assert.Equal(t, 2, len(routes[0].Route.Filters))
	assert.Equal(t, "www.backend.com", routes[0].Route.Endpoint)
}

func TestEskipToInnkeeperMinimal(t *testing.T) {
	route := []*eskip.Route{{
		Id:     "theid",
		Method: "GET"}}

	routes := convertEskipToInnkeeper(route)

	assert.Equal(t, 1, len(routes))
	assert.Equal(t, "theid", routes[0].Name)
	assert.Equal(t, "GET", routes[0].Route.Matcher.MethodMatcher)
	assert.Equal(t, 0, len(routes[0].Route.Filters))
	assert.Equal(t, 0, len(routes[0].Route.Matcher.HeaderMatchers))
	assert.Equal(t, "", routes[0].Route.Endpoint)
}
