// Package innkeeper implements a data client for the Innkeeper API.
package innkeeper

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zalando/skipper/skipper"
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

type routeDoc string

type Client struct {
	baseUrl      *url.URL
	auth         Authentication
	authToken    string
	lastModified string
	httpClient   *http.Client
	dataChan     chan skipper.RawData

	// store the routes for comparison during
	// the subsequent polls
	doc    map[int64]string
	closer chan interface{}
	closed chan interface{}
}

type Options struct {
	Address        string
	Insecure       bool
	PollTimeout    time.Duration
	Authentication Authentication
}

// Creates an innkeeper client.
func Make(o Options) (*Client, error) {
	u, err := url.Parse(o.Address)
	if err != nil {
		return nil, err
	}

	c := &Client{
		u, o.Authentication, "", "",
		&http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: o.Insecure}}},
		make(chan skipper.RawData),
		make(map[int64]string),
		make(chan interface{}),
		make(chan interface{})}

	// start a polling loop
	go func() {
		to := time.Duration(0)
		for {
			select {
			case <-time.After(to):
				to = o.PollTimeout
				if c.poll() {
					c.dataChan <- toDocument(c.doc)
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
	switch authErrorType(error) {
	case authErrorPermission, authErrorAuthentication:
		return true
	default:
		return false
	}
}

func (c *Client) authenticate() error {
	t, err := c.auth.Token()
	if err != nil {
		return err
	}

	c.authToken = t
	return nil
}

func (c *Client) requestData(authRetry bool, url string) ([]*routeData, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", c.authToken)
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	if response.StatusCode == 401 {
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

	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to receive data: %s", response.Status)
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
	return fmt.Sprintf("route%d", d.Id)
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
	if len(d.Route.MatchMethods) == 1 {
		m = appendFormat(m, `Method("%s")`, d.Route.MatchMethods[0])
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

func getFilterExpressions(d *routeData) string {
	f := []string{}

	if d.Route.RewritePath != nil {
		rx := d.Route.RewritePath.Match
		if rx == "" {
			rx = ".*"
		}

		f = appendFormat(f, `pathRewrite(/%s/, "%s")`, rx, d.Route.RewritePath.Replace)
	}

	f = appendFormatHeaders(f, `requestHeader("%s", "%s")`, d.Route.RequestHeaders, false)
	f = appendFormatHeaders(f, `responseHeader("%s", "%s")`, d.Route.ResponseHeaders, false)

	if len(f) == 0 {
		return ""
	}

	f = append([]string{""}, f...)
	return strings.Join(f, " -> ")
}

func getEndpointAddress(d *routeData) string {
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
	f := getFilterExpressions(d)
	a := getEndpointAddress(d)
	return fmt.Sprintf(`%s: %s%s -> "%s"`, key, m, f, a)
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
			if c.doc[di.Id] != "" {
				delete(c.doc, di.Id)
				updated = true
			}
		default:
			docEntry := c.routeJsonToEskip(di)
			if c.doc[di.Id] != docEntry {
				c.doc[di.Id] = docEntry
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
	)

	if len(c.doc) == 0 {
		url := c.createUrl(allRoutesPathRoot)
		d, err = c.requestData(true, url)
	} else {
		url := c.createUrl(updatePathRoot, c.lastModified)
		d, err = c.requestData(true, url)
	}

	if err != nil {
		log.Println("error while receiving innkeeper data;", err)
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
func toDocument(doc map[int64]string) routeDoc {
	var d []byte
	for _, r := range doc {
		d = append(d, []byte(r)...)
		d = append(d, ';', '\n')
	}

	return routeDoc(d)
}

// returns skipper raw data value
func (c *Client) Receive() <-chan skipper.RawData { return c.dataChan }

// Stops polling, but only after the last update is consumed on the receive channel.
func (c *Client) Close() {
	close(c.closer)
}

// returns eskip format of the route doc
//
// todo: since only the routes are stored in the
// RawData interface, no need for it, it can be
// just a string
func (d routeDoc) Get() string { return string(d) }
