// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Package innkeeper implements a DataClient for reading skipper route
definitions from an Innkeeper service.

(See the DataClient interface in the skipper/routing package.)

Innkeeper is a service to maintain large sets of routes in a multi-user
environment with OAuth2 authentication and permission scopes.

See: https://github.com/zalando/innkeeper
*/
package innkeeper

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zalando/skipper/eskip"
	"io"
	"io/ioutil"
	"net/http"
)

const (
	allRoutesPath = "/routes"
	updatePathFmt = "/updated-routes/%s"
	bearerPrefix  = "Bearer "
)

type (
	matchType     string
	authErrorType string
)

const (
	authHeaderName = "Authorization"

	matchStrict = matchType("STRICT")
	matchRegex  = matchType("REGEX")

	authErrorPermission     = authErrorType("AUTH1")
	authErrorAuthentication = authErrorType("AUTH2")

	allRoutesPathRoot = "routes"
	updatePathRoot    = "updated-routes"

	fixedRedirectStatus = http.StatusFound
)

// json serialization object for innkeeper route definitions
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

	matcher struct {
		HostMatcher    string          `json:"host_matcher,omitempty"`
		PathMatcher    *pathMatcher    `json:"path_matcher,omitempty"`
		MethodMatcher  string          `json:"method_matcher,omitempty"`
		HeaderMatchers []headerMatcher `json:"header_matchers"`
	}

	filter struct {
		Name string        `json:"name,omitempty"`
		Args []interface{} `json:"args"`
	}

	routeDef struct {
		Matcher  matcher  `json:"matcher,omitempty"`
		Filters  []filter `json:"filters"`
		Endpoint string   `json:"endpoint,omitempty"`
	}

	routeData struct {
		Id         int64    `json:"id,omitempty"`
		Name       string   `json:"name,omitempty"`
		ActivateAt string   `json:"activate_at,omitempty"`
		CreatedAt  string   `json:"created_at,omitempty"`
		DeletedAt  string   `json:"deleted_at,omitempty"`
		Route      routeDef `json:"route"`
	}
)

// json serialization object for innkeeper error messages
type apiError struct {
	ErrorType string `json:"type"`
	Status    int32  `json:"status"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
}

// Initialization options for the Innkeeper client.
type Options struct {

	// The network address where the Innkeeper API is accessible.
	Address string

	// When true, TLS certificate verification is skipped.
	Insecure bool

	// Authentication to be used when connecting to Innkeeper.
	Authentication Authentication

	// An eskip filter chain expression to prepend to each route loaded
	// from Innkeeper. (E.g. "filter1() -> filter2() -> filter3()")
	PreRouteFilters string

	// An eskip filter chain expression to append to each route loaded
	// from Innkeeper. (E.g. "filter1() -> filter2() -> filter3()")
	PostRouteFilters string
}

// A Client is used to load the whole set of routes and the updates from an
// Innkeeper service.
type Client struct {
	opts             Options
	preRouteFilters  []*eskip.Filter
	postRouteFilters []*eskip.Filter
	authToken        string
	httpClient       *http.Client
	lastChanged      string
}

// Returns a new Client.
func New(o Options) (*Client, error) {
	preFilters, err := eskip.ParseFilters(o.PreRouteFilters)
	if err != nil {
		return nil, err
	}

	postFilters, err := eskip.ParseFilters(o.PostRouteFilters)
	if err != nil {
		return nil, err
	}

	return &Client{
		opts:             o,
		preRouteFilters:  preFilters,
		postRouteFilters: postFilters,
		httpClient: &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: o.Insecure}}}}, nil
}

// Converts Innkeeper header conditions to a header map.
func convertHeaders(d *routeData) (map[string]string, map[string][]string) {
	hs := make(map[string]string)
	hrs := make(map[string][]string)
	for _, h := range d.Route.Matcher.HeaderMatchers {
		if h.Typ == matchStrict {
			hs[h.Name] = h.Value
		} else {
			hrs[h.Name] = []string{h.Value}
		}
	}

	return hs, hrs
}

// Converts the Innkeeper filter objects in a route definition to their eskip
// representation.
func convertFilters(d *routeData) []*eskip.Filter {
	var fs []*eskip.Filter

	for _, h := range d.Route.Filters {
		fs = append(fs, &eskip.Filter{
			Name: h.Name,
			Args: h.Args})
	}

	return fs
}

// Converts an Innkeeper route definition to its eskip representation.
func convertRoute(id string, d *routeData, preRouteFilters, postRouteFilters []*eskip.Filter) *eskip.Route {
	var p string
	var prx []string

	if d.Route.Matcher.PathMatcher != nil {
		if d.Route.Matcher.PathMatcher.Typ == matchStrict {
			p = d.Route.Matcher.PathMatcher.Match
		}

		if d.Route.Matcher.PathMatcher.Typ == matchRegex {
			prx = []string{d.Route.Matcher.PathMatcher.Match}
		}
	}

	var hst []string
	if d.Route.Matcher.HostMatcher != "" {
		hst = []string{d.Route.Matcher.HostMatcher}
	}

	m := d.Route.Matcher.MethodMatcher
	hs, hrs := convertHeaders(d)

	fs := preRouteFilters
	fs = append(fs, convertFilters(d)...)
	fs = append(fs, postRouteFilters...)

	return &eskip.Route{
		Id:            id,
		HostRegexps:   hst,
		Path:          p,
		PathRegexps:   prx,
		Method:        m,
		Headers:       hs,
		HeaderRegexps: hrs,
		Filters:       fs,
		Shunt:         d.Route.Endpoint == "",
		Backend:       d.Route.Endpoint}
}

// Converts a set of Innkeeper route definitions to their eskip representation.
func convertJsonToEskip(data []*routeData, preRouteFilters, postRouteFilters []*eskip.Filter) ([]*eskip.Route, []string, string) {
	var (
		routes      []*eskip.Route
		deleted     []string
		lastChanged string
	)

	for _, d := range data {
		id := d.Name
		if id == "" {
			id = fmt.Sprintf("route%d", d.Id)
		}

		if d.DeletedAt != "" {
			if d.DeletedAt > lastChanged {
				lastChanged = d.DeletedAt
			}

			deleted = append(deleted, id)
			continue
		}

		if d.CreatedAt > lastChanged {
			lastChanged = d.CreatedAt
		}

		routes = append(routes, convertRoute(id, d, preRouteFilters, postRouteFilters))
	}

	return routes, deleted, lastChanged
}

// Parses an Innkeeper API error message and returns its type.
func parseApiError(r io.Reader) (string, error) {
	message, err := ioutil.ReadAll(r)

	if err != nil {
		return "", err
	}

	ae := apiError{}
	if err := json.Unmarshal(message, &ae); err != nil {
		return "", err
	}

	return ae.ErrorType, nil
}

// Checks whether an API error is authentication/authorization related.
func isApiAuthError(error string) bool {
	aerr := authErrorType(error)
	return aerr == authErrorPermission || aerr == authErrorAuthentication
}

// Authenticates a client and stores the authentication token.
func (c *Client) authenticate() error {
	if c.opts.Authentication == nil {
		c.authToken = ""
		return nil
	}

	t, err := c.opts.Authentication.GetToken()
	if err != nil {
		return err
	}

	c.authToken = t
	return nil
}

// Checks if an http response status indicates an error, and returns an error
// object if it does.
func getHttpError(r *http.Response) (error, bool) {
	switch {
	case r.StatusCode < http.StatusBadRequest:
		return nil, false
	case r.StatusCode < http.StatusInternalServerError:
		return fmt.Errorf("innkeeper request failed: %s", r.Status), true
	case r.StatusCode < 600:
		return fmt.Errorf("innkeeper error: %s", r.Status), true
	default:
		return fmt.Errorf("unexpected error: %d - %s", r.StatusCode, r.Status), true
	}
}

func setAuthToken(h http.Header, value string) {
	h.Set(authHeaderName, bearerPrefix+value)
}

func (c *Client) writeRoute(url string, route *routeData) error {

	res, err := json.Marshal(route)

	req, err := http.NewRequest("POST", url, bytes.NewReader(res))

	if err != nil {
		return err
	}

	authToken, err := c.opts.Authentication.GetToken()

	if err != nil {
		return err
	}

	setAuthToken(req.Header, authToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusBadRequest {
		apiError, err := parseApiError(response.Body)
		if err != nil {
			return err
		}

		return fmt.Errorf("unknown error: %s", apiError)
	}

	return nil
}

// Calls an http request to an Innkeeper URL for route definitions.
// If authRetry is true, and the request fails due to an
// authentication/authorization related problem, it retries the request with
// reauthenticating first.
func (c *Client) requestData(authRetry bool, url string) ([]*routeData, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	setAuthToken(req.Header, c.authToken)
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		apiError, err := parseApiError(response.Body)
		if err != nil {
			return nil, err
		}

		if !isApiAuthError(apiError) {
			return nil, fmt.Errorf("unknown authentication error: %s", apiError)
		}

		if !authRetry {
			return nil, errors.New("authentication failed")
		}

		err = c.authenticate()
		if err != nil {
			return nil, err
		}

		return c.requestData(false, url)
	}

	if err, hasErr := getHttpError(response); hasErr {
		return nil, err
	}

	routesData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	result := []*routeData{}
	err = json.Unmarshal(routesData, &result)
	return result, err
}

func (c *Client) LoadAndParseAll() ([]*eskip.RouteInfo, error) {
	routeList, err := c.LoadAll()
	if err != nil {
		return nil, err
	}

	routeInfos := []*eskip.RouteInfo{}

	for _, route := range routeList {
		routeInfo := &eskip.RouteInfo{
			*route, nil}
		routeInfos = append(routeInfos, routeInfo)
	}

	return routeInfos, nil
}

// Returns all active routes from Innkeeper.
func (c *Client) LoadAll() ([]*eskip.Route, error) {
	d, err := c.requestData(true, c.opts.Address+allRoutesPath)
	if err != nil {
		return nil, err
	}

	routes, _, lastChanged := convertJsonToEskip(d, c.preRouteFilters, c.postRouteFilters)
	if lastChanged > c.lastChanged {
		c.lastChanged = lastChanged
	}

	return routes, nil
}

// Returns all new and deleted routes from Innkeeper since the last LoadAll request.
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	d, err := c.requestData(true, c.opts.Address+fmt.Sprintf(updatePathFmt, c.lastChanged))
	if err != nil {
		return nil, nil, err
	}

	routes, deleted, lastChanged := convertJsonToEskip(d, c.preRouteFilters, c.postRouteFilters)
	if lastChanged > c.lastChanged {
		c.lastChanged = lastChanged
	}

	return routes, deleted, nil
}

func (c *Client) UpsertAll(routes []*eskip.Route) error {
	// convert the routes to the innkeeper json structs
	data := convertEskipToInnkeeper(routes)

	for _, route := range data {
		err := c.writeRoute(c.opts.Address+allRoutesPath, route)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) DeleteAllIf(routes []*eskip.Route, cond eskip.RoutePredicate) error {
	return nil
}
