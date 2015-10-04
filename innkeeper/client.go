// Package innkeeper implements a data client for the Innkeeper API.
package innkeeper

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type (
	pathMatchType string
	endpointType  string
	authErrorType string
)

const (
	authHeaderName    = "Authorization"
	fixRedirectStatus = http.StatusFound

	pathMatchStrict = pathMatchType("STRICT")
	pathMatchRegexp = pathMatchType("REGEXP")

	endpointReverseProxy      = endpointType("REVERSE_PROXY")
	endpointPermanentRedirect = endpointType("PERMANENT_REDIRECT")

	authErrorPermission     = authErrorType("AUTH1")
	authErrorAuthentication = authErrorType("AUTH2")

	allRoutesPathRoot = "routes"
	updatePathRoot    = "updated-routes"
)

// todo: implement this either with the oauth service
// or a token passed in from a command line option
type Authentication interface {
	Token() (string, error)
}

// Provides an Authentication implementation with fixed token string
type FixedToken string

func (fa FixedToken) Token() (string, error) { return string(fa), nil }

type routeCache map[int64]map[string]string

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

type Client struct {
	baseUrl          *url.URL
	auth             Authentication
	authToken        string
	lastModified     string
	preRouteFilters  []string
	postRouteFilters []string
	httpClient       *http.Client
	dataChan         chan string
	routeCache       routeCache
	closer           chan interface{}
}

type Options struct {
	Address          string
	Insecure         bool
	PollTimeout      time.Duration
	Authentication   Authentication
	PreRouteFilters  []string
	PostRouteFilters []string
}

// Creates an innkeeper client.
func New(o Options) (*Client, error) {
	u, err := url.Parse(o.Address)
	if err != nil {
		return nil, err
	}

	c := &Client{
		u, o.Authentication, "", "",
		o.PreRouteFilters, o.PostRouteFilters,
		&http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: o.Insecure}}},
		make(chan string),
		make(routeCache),
		make(chan interface{})}

	// start a polling loop
	go func() {
		to := time.Duration(0)
		for {
			select {
			case <-time.After(to):
				to = o.PollTimeout
				if c.poll() {
					c.dataChan <- toDocument(c.routeCache)
				}
			case <-c.closer:
				return
			}
		}
	}()

	return c, nil
}

func (c *Client) createUrl(path ...string) string {
	u := *c.baseUrl
	u.Path = strings.Join(append([]string{""}, path...), "/")
	return (&u).String()
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
	t, err := c.auth.Token()
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

func getRouteKey(d *routeData) string {
	return fmt.Sprintf("route%d%s", d.Id, d.Route.matchMethod)
}

func appendFormat(exps []string, format string, args ...interface{}) []string {
	return append(exps, fmt.Sprintf(format, args...))
}

func appendFormatHeaders(exps []string, format string, defs []headerData, canonicalize bool) []string {
	for _, d := range defs {
		key := d.Name
		if canonicalize {
			key = http.CanonicalHeaderKey(key)
		}

		exps = appendFormat(exps, format, key, d.Value)
	}

	return exps
}

func getMatcherExpression(d *routeData) string {
	m := []string{}

	// there can be only 0 or 1 here, because routes for multiple methods
	// have been already split
	if d.Route.matchMethod != "" {
		m = appendFormat(m, `Method("%s")`, d.Route.matchMethod)
	}

	m = appendFormatHeaders(m, `Header("%s", "%s")`, d.Route.MatchHeaders, true)

	switch d.Route.MatchPath.Typ {
	case pathMatchStrict:
		m = appendFormat(m, `Path("%s")`, d.Route.MatchPath.Match)
	case pathMatchRegexp:
		m = appendFormat(m, `PathRegexp("%s")`, d.Route.MatchPath.Match)
	}

	if len(m) == 0 {
		m = []string{"Any()"}
	}

	return strings.Join(m, " && ")
}

func innkeeperProtocolToScheme(protocol string) string {
	return strings.ToLower(protocol)
}

func isRedirectEndpoint(d *routeData) bool {
	return d.Route.Endpoint.Typ == endpointPermanentRedirect
}

func getFilterExpressions(d *routeData, preRouteFilters, postRouteFilters []string) string {
	f := preRouteFilters

	if d.Route.RewritePath != nil {
		rx := d.Route.RewritePath.Match
		if rx == "" {
			rx = ".*"
		}

		f = appendFormat(f, `pathRewrite(/%s/, "%s")`, rx, d.Route.RewritePath.Replace)
	}

	f = appendFormatHeaders(f, `requestHeader("%s", "%s")`, d.Route.RequestHeaders, false)
	f = appendFormatHeaders(f, `responseHeader("%s", "%s")`, d.Route.ResponseHeaders, false)

	if isRedirectEndpoint(d) {
		f = appendFormat(f, `redirect(%d, "%s")`, fixRedirectStatus, (&url.URL{
			Scheme: innkeeperProtocolToScheme(d.Route.Endpoint.Protocol),
			Host:   fmt.Sprintf("%s:%d", d.Route.Endpoint.Hostname, d.Route.Endpoint.Port),
			Path:   d.Route.Endpoint.Path}).String())
	}

	f = append(f, postRouteFilters...)

	if len(f) == 0 {
		return ""
	}

	f = append([]string{""}, f...)
	return strings.Join(f, " -> ")
}

func getEndpointAddress(d *routeData) string {
	if isRedirectEndpoint(d) {
		return "<shunt>"
	}

	a := url.URL{
		Scheme: d.Route.Endpoint.Protocol,
		Host:   fmt.Sprintf("%s:%d", d.Route.Endpoint.Hostname, d.Route.Endpoint.Port)}
	if a.Scheme == "" {
		a.Scheme = "https"
	}

	return a.String()
}

func (c *Client) routeJsonToEskip(d *routeData) string {
	key := getRouteKey(d)
	m := getMatcherExpression(d)
	f := getFilterExpressions(d, c.preRouteFilters, c.postRouteFilters)
	a := getEndpointAddress(d)
	return fmt.Sprintf(`%s: %s%s -> "%s"`, key, m, f, a)
}

func (c *Client) convertToEntries(r *routeData) map[string]string {
	split := make(map[string]string)
	if len(r.Route.MatchMethods) == 0 {
		split["_"] = c.routeJsonToEskip(r)
		return split
	}

	for _, m := range r.Route.MatchMethods {
		copy := r.Route
		copy.matchMethod = m
		split[m] = c.routeJsonToEskip(&routeData{r.Id, r.CreatedAt, r.DeletedAt, copy})
	}

	return split
}

// updates the route doc from received data, and
// returns true if there were any changes, otherwise
// false.
func (c *Client) updateDoc(d []*routeData) bool {
	updated := false
	for _, di := range d {
		if di.DeletedAt > c.lastModified {
			c.lastModified = di.DeletedAt
		} else if di.CreatedAt > c.lastModified {
			c.lastModified = di.CreatedAt
		}

		switch {
		case di.DeletedAt != "":
			if _, exists := c.routeCache[di.Id]; exists {
				delete(c.routeCache, di.Id)
				updated = true
			}
		default:
			entriesPerMethod := c.convertToEntries(di)
			if stored, exists := c.routeCache[di.Id]; exists {
				for m, entry := range entriesPerMethod {
					if stored[m] != entry {
						stored[m] = entry
						updated = true
					}
				}

				for m, _ := range stored {
					if _, exists := entriesPerMethod[m]; !exists {
						delete(stored, m)
						updated = true
					}
				}
			} else {
				c.routeCache[di.Id] = entriesPerMethod
				updated = true
			}
		}
	}

	return updated
}

func (c *Client) poll() bool {
	var (
		d   []*routeData
		err error
		url string
	)

	if len(c.routeCache) == 0 {
		url = c.createUrl(allRoutesPathRoot)
	} else {
		url = c.createUrl(updatePathRoot, c.lastModified)
	}
	d, err = c.requestData(true, url)

	if err != nil {
		log.Println("error while receiving innkeeper data", err)
		return false
	}

	if len(d) == 0 {
		return false
	}

	updated := c.updateDoc(d)
	if updated {
		log.Println("routing doc updated from innkeeper")
	}

	return updated
}

// returns eskip format
func toDocument(c routeCache) string {
	var d []byte
	for _, mr := range c {
		for _, r := range mr {
			d = append(d, []byte(r)...)
			d = append(d, ';', '\n')
		}
	}

	return string(d)
}

// returns skipper raw data value
func (c *Client) Receive() <-chan string { return c.dataChan }

// Stops polling, but only after the last update is consumed on the receive channel.
func (c *Client) Close() {
	close(c.closer)
}
