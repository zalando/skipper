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

(See the DataClient interface in the github.com/zalando/skipper/routing
package.)

Innkeeper is a service to maintain large sets of routes in a multi-user
environment with OAuth2 authentication and permission scopes.

See: https://github.com/zalando/innkeeper
*/
package innkeeper

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

const (
	allRoutesPath = "/routes"
	updatePathFmt = "/updated-routes/%s"
)

type (
	pathMatchType string
	endpointType  string
	authErrorType string
)

const (
	authHeaderName = "Authorization"

	pathMatchStrict = pathMatchType("STRICT")
	pathMatchRegexp = pathMatchType("REGEXP")

	endpointReverseProxy      = endpointType("REVERSE_PROXY")
	endpointPermanentRedirect = endpointType("PERMANENT_REDIRECT")

	authErrorPermission     = authErrorType("AUTH1")
	authErrorAuthentication = authErrorType("AUTH2")

	allRoutesPathRoot = "routes"
	updatePathRoot    = "updated-routes"

	fixedRedirectStatus = http.StatusFound
)

// json serialization object for innkeeper route definitions
type (
	pathMatch struct {
		Typ   pathMatchType `json:"type"`
		Match string        `json:"match"`
	}

	pathRewrite struct {
		Match   string `json:"match"`
		Replace string `json:"replace"`
	}

	headerData struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}

	endpoint struct {
		Typ      endpointType `json:"type"`
		Protocol string       `json:"protocol"`
		Hostname string       `json:"hostname"`
		Port     int          `json:"port"`
		Path     string       `json:"path"`
	}

	routeDef struct {
		// non-parsed field
		matchMethod string

		MatchMethods    []string     `json:"match_methods"`
		MatchHeaders    []headerData `json:"match_headers"`
		MatchPath       pathMatch    `json:"match_path"`
		RewritePath     *pathRewrite `json:"rewrite_path"`
		RequestHeaders  []headerData `json:"request_headers"`
		ResponseHeaders []headerData `json:"response_headers"`
		Endpoint        endpoint     `json:"endpoint"`
	}

	routeData struct {
		Id        int64    `json:"id"`
		CreatedAt string   `json:"createdAt"`
		DeletedAt string   `json:"deletedAt"`
		Route     routeDef `json:"route"`
	}
)

// json serialization object for innkeeper error messages
type apiError struct {
	Message   string `json:"message"`
	ErrorType string `json:"type"`
}

// An Authentication object provides authentication to Innkeeper.
type Authentication interface {
	GetToken() (string, error)
}

// A FixedToken provides Innkeeper authentication by an unchanged token
// string.
type FixedToken string

// Returns the fixed token.
func (ft FixedToken) GetToken() (string, error) { return string(ft), nil }

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

// Generate a separate route for each method if an Innkeeper route contains
// more than one method condition, because eskip routes can contain only a
// single method condition.
func splitOnMethods(data []*routeData) []*routeData {
	var split []*routeData
	for _, d := range data {
		if len(d.Route.MatchMethods) == 0 {
			split = append(split, d)
			continue
		}

		for _, m := range d.Route.MatchMethods {
			copy := d.Route
			copy.matchMethod = m
			split = append(split, &routeData{d.Id, d.CreatedAt, d.DeletedAt, copy})
		}
	}

	return split
}

// Converts Innkeeper header conditions to a header map.
func convertHeaders(d *routeData) map[string]string {
	hs := make(map[string]string)
	for _, h := range d.Route.MatchHeaders {
		hs[h.Name] = h.Value
	}

	return hs
}

// In the current API specification of Innkeeper, a protocol can be HTTPS or
// HTTP.
func innkeeperProcotolToScheme(p string) string {
	return strings.ToLower(p)
}

// Converts an Innkeeper endpoint structure into an endpoint address.
func innkeeperEndpointToUrl(e *endpoint, withPath bool) string {
	scheme := innkeeperProcotolToScheme(e.Protocol)
	host := fmt.Sprintf("%s:%d", e.Hostname, e.Port)
	u := &url.URL{Scheme: scheme, Host: host}

	if withPath {
		u.Path = e.Path
	}

	return u.String()
}

// Converts the Innkeeper filter objects in a route definition to their eskip
// representation.
func convertFilters(d *routeData) []*eskip.Filter {
	var fs []*eskip.Filter

	if d.Route.RewritePath != nil {
		rx := d.Route.RewritePath.Match
		if rx == "" {
			rx = ".*"
		}

		fs = append(fs, &eskip.Filter{
			filters.ModPathName,
			[]interface{}{rx, d.Route.RewritePath.Replace}})
	}

	for _, h := range d.Route.RequestHeaders {
		fs = append(fs, &eskip.Filter{
			filters.RequestHeaderName,
			[]interface{}{h.Name, h.Value}})
	}

	for _, h := range d.Route.ResponseHeaders {
		fs = append(fs, &eskip.Filter{
			filters.ResponseHeaderName,
			[]interface{}{h.Name, h.Value}})
	}

	if d.Route.Endpoint.Typ == endpointPermanentRedirect {
		fs = append(fs, &eskip.Filter{
			filters.RedirectName,
			[]interface{}{
				fixedRedirectStatus,
				innkeeperEndpointToUrl(&d.Route.Endpoint, true)}})
	}

	return fs
}

// Converts an Innkeeper backend to an eskip endpoint address or a shunt.
func convertBackend(d *routeData) (bool, string) {
	var backend string
	shunt := d.Route.Endpoint.Typ == endpointPermanentRedirect
	if !shunt {
		backend = innkeeperEndpointToUrl(&d.Route.Endpoint, false)
	}

	return shunt, backend
}

// Converts an Innkeeper route definition to its eskip representation.
func convertRoute(id string, d *routeData, preRouteFilters, postRouteFilters []*eskip.Filter) *eskip.Route {
	var p string
	if d.Route.MatchPath.Typ == pathMatchStrict {
		p = d.Route.MatchPath.Match
	}

	var prx []string
	if d.Route.MatchPath.Typ == pathMatchRegexp {
		prx = []string{d.Route.MatchPath.Match}
	}

	m := d.Route.matchMethod
	hs := convertHeaders(d)

	fs := preRouteFilters
	fs = append(fs, convertFilters(d)...)
	fs = append(fs, postRouteFilters...)

	shunt, backend := convertBackend(d)

	return &eskip.Route{
		Id:          id,
		Path:        p,
		PathRegexps: prx,
		Method:      m,
		Headers:     hs,
		Filters:     fs,
		Shunt:       shunt,
		Backend:     backend}
}

// Converts a set of Innkeeper route definitions to their eskip representation.
func convertData(data []*routeData, preRouteFilters, postRouteFilters []*eskip.Filter) ([]*eskip.Route, []string, string) {
	var (
		routes      []*eskip.Route
		deleted     []string
		lastChanged string
	)

	data = splitOnMethods(data)
	for _, d := range data {
		id := fmt.Sprintf("route%d%s", d.Id, d.Route.matchMethod)

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

// Calls an http request to an Innkeeper URL for route definitions.
// If authRetry is true, and the request fails due to an
// authentication/authorization related problem, it retries the request with
// reauthenticating first.
func (c *Client) requestData(authRetry bool, url string) ([]*routeData, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add(authHeaderName, c.authToken)
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

// Returns all active routes from Innkeeper.
func (c *Client) GetInitial() ([]*eskip.Route, error) {
	d, err := c.requestData(true, c.opts.Address+allRoutesPath)
	if err != nil {
		return nil, err
	}

	routes, _, lastChanged := convertData(d, c.preRouteFilters, c.postRouteFilters)
	if lastChanged > c.lastChanged {
		c.lastChanged = lastChanged
	}

	return routes, nil
}

// Returns all new and deleted routes from Innkeeper since the last GetInitial request.
func (c *Client) GetUpdate() ([]*eskip.Route, []string, error) {
	d, err := c.requestData(true, c.opts.Address+fmt.Sprintf(updatePathFmt, c.lastChanged))
	if err != nil {
		return nil, nil, err
	}

	routes, deleted, lastChanged := convertData(d, c.preRouteFilters, c.postRouteFilters)
	if lastChanged > c.lastChanged {
		c.lastChanged = lastChanged
	}

	return routes, deleted, nil
}
