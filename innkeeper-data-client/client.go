package innkeeper

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	allRoutesPath         = "/routes"
	updatePathFmt         = "/updated-routes/%s"
	receiveInitialTimeout = 1200 * time.Millisecond
    defaultPollTimeout = 30 * time.Millisecond
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

type pathMatch struct {
	Typ   pathMatchType `json:"type"`
	Match string        `json:"match"`
}

type pathRewrite struct {
	Match   string `json:"match"`
	Replace string `json:"replace"`
}

type headerData struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type endpoint struct {
	Typ      endpointType `json:"type"`
	Protocol string       `json:"protocol"`
	Hostname string       `json:"hostname"`
	Port     int          `json:"port"`
	Path     string       `json:"path"`
}

type routeDef struct {
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

type routeData struct {
	Id        int64    `json:"id"`
	CreatedAt string   `json:"createdAt"`
	DeletedAt string   `json:"deletedAt"`
	Route     routeDef `json:"route"`
}

type apiError struct {
	Message   string `json:"message"`
	ErrorType string `json:"type"`
}

type Authentication interface {
	Token() (string, error)
}

type Options struct {
	Address          string
	Insecure         bool
	PollTimeout      time.Duration
	Authentication   Authentication
	PreRouteFilters  string
	PostRouteFilters string
}

type Client struct {
	opts             Options
	preRouteFilters  []*eskip.Filter
	postRouteFilters []*eskip.Filter
	initial          chan []*eskip.Route
	updates          chan *routing.DataUpdate
	authToken        string
	httpClient       *http.Client
	lastChanged      string
}

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

func convertHeaders(d *routeData) map[string]string {
	hs := make(map[string]string)
	for _, h := range d.Route.MatchHeaders {
		hs[h.Name] = h.Value
	}

	return hs
}

func innkeeperProcotolToScheme(p string) string {
	return strings.ToLower(p)
}

func innkeeperEndpointToUrl(e *endpoint, withPath bool) string {
	scheme := innkeeperProcotolToScheme(e.Protocol)
	host := fmt.Sprintf("%s:%d", e.Hostname, e.Port)
	u := &url.URL{Scheme: scheme, Host: host}

	if withPath {
		u.Path = e.Path
	}

	return u.String()
}

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

func convertBackend(d *routeData) (bool, string) {
	var backend string
	shunt := d.Route.Endpoint.Typ == endpointPermanentRedirect
	if !shunt {
		backend = innkeeperEndpointToUrl(&d.Route.Endpoint, false)
	}

	return shunt, backend
}

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

func isApiAuthError(error string) bool {
	aerr := authErrorType(error)
	return aerr == authErrorPermission || aerr == authErrorAuthentication
}

func (c *Client) authenticate() error {
	if c.opts.Authentication == nil {
		c.authToken = ""
		return nil
	}

	t, err := c.opts.Authentication.Token()
	if err != nil {
		return err
	}

	c.authToken = t
	return nil
}

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

func (c *Client) receiveUpdates() {
	for {
		time.Sleep(c.opts.PollTimeout)
		d, err := c.requestData(true, c.opts.Address+fmt.Sprintf(updatePathFmt, c.lastChanged))
		if err != nil {
			log.Println("error while receiving innkeeper data", err)
			continue
		}

		routes, deleted, lastChanged := convertData(d, c.preRouteFilters, c.postRouteFilters)
		if lastChanged > c.lastChanged {
			c.lastChanged = lastChanged
		}

		if len(routes) > 0 || len(deleted) > 0 {
			c.updates <- &routing.DataUpdate{routes, deleted}
		}
	}
}

func (c *Client) receiveInitial() {
	failedFirst := false
	for {
		d, err := c.requestData(true, c.opts.Address+allRoutesPath)
		if err != nil {
			failedFirst = true
			log.Println("error while receiving innkeeper data", err)
			time.Sleep(c.opts.PollTimeout)
			continue
		}

		routes, _, lastChanged := convertData(d, c.preRouteFilters, c.postRouteFilters)
		if lastChanged > c.lastChanged {
			c.lastChanged = lastChanged
		}

		if failedFirst {
			c.updates <- &routing.DataUpdate{routes, nil}
		} else {
			c.initial <- routes
		}

		go c.receiveUpdates()
		return
	}
}

func New(o Options) (*Client, error) {
    if o.PollTimeout <= 0 {
        o.PollTimeout = defaultPollTimeout
    }

    preFilters, err := eskip.ParseFilters(o.PreRouteFilters)
    if err != nil {
        return nil, err
    }

    postFilters, err := eskip.ParseFilters(o.PostRouteFilters)
    if err != nil {
        return nil, err
    }

	c := &Client{
		opts:    o,
        preRouteFilters: preFilters,
        postRouteFilters: postFilters,
		initial: make(chan []*eskip.Route),
		updates: make(chan *routing.DataUpdate),
		httpClient: &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: o.Insecure}}}}
	go c.receiveInitial()
	return c, nil
}

func (c *Client) Receive() ([]*eskip.Route, <-chan *routing.DataUpdate) {
	var routes []*eskip.Route
	select {
	case routes = <-c.initial:
	case <-time.After(receiveInitialTimeout):
		log.Println("timeout while receiving initial set of routes from innkeeper")
	}

	return routes, c.updates
}
