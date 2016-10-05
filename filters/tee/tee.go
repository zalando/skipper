package tee

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	Name = "Tee"
)

type teeSpec struct{}

type teeType int

const (
	asBackend teeType = iota + 1
	pathModified
)

type tee struct {
	client      *http.Client
	typ         teeType
	host        string
	scheme      string
	rx          *regexp.Regexp
	replacement string
}

type teeTie struct {
	r io.Reader
	w *io.PipeWriter
}

// Returns a new tee filter Spec, whose instances execute
// the exact same Request against a shadow backend.
// parameters: shadow backend url, optional - the path(as a regexp) to match and the replacement string.
// Name: "tee".
// Example
// Path("/api/v3") -> tee("https://api.example.com") -> "http://example.org/"
// This route wil send incoming  request to http://example.org/api/v3 but will also send
// a copy of the query to https://api.example.com/api/v3.
// Example
// Path("/api/v3") -> tee("https://api.example.com", ".*", "/v1/" ) -> "http://example.org/"
// This route wil send incoming request to http://example.org/api/v3 but will also send
// a copy of the request to https://api.example.com/v1/ . Note that scheme and path are changed
func NewTee() *teeSpec {
	return &teeSpec{}
}

func (tt *teeTie) Read(b []byte) (int, error) {
	n, err := tt.r.Read(b)

	if err != nil && err != io.EOF {
		tt.w.CloseWithError(err)
		return n, err
	}

	if n > 0 {
		if _, werr := tt.w.Write(b[:n]); werr != nil {
			log.Error("error while tee request", werr)
		}
	}

	if err == io.EOF {
		tt.w.Close()
	}

	return n, err
}

func (tt *teeTie) Close() error { return nil }

//We do not touch response at all
func (r *tee) Response(filters.FilterContext) {}

// Request is copied and then modified to adopt changes in new backend
func (r *tee) Request(fc filters.FilterContext) {
	copyOfRequest := cloneRequest(r, fc.Request())
	go func() {
		rsp, err := r.client.Do(&copyOfRequest)
		if err != nil {
			log.Warn("error while tee request", err)
		}
		if err == nil {
			defer rsp.Body.Close()
		}
	}()
}

// copies requests changes URL and Host in request.
// If 2nd and 3rd params are given path is also modified by applying regexp
func cloneRequest(t *tee, req *http.Request) http.Request {
	clone := new(http.Request)
	*clone = *req
	copyUrl := new(url.URL)
	*copyUrl = *req.URL
	clone.URL = copyUrl
	clone.URL.Host = t.host
	clone.URL.Scheme = t.scheme
	clone.Host = t.host

	pr, pw := io.Pipe()
	tr := &teeTie{req.Body, pw}
	clone.Body = pr
	req.Body = tr
	//Setting to empty string otherwise go-http doesn't allow having it in client request
	clone.RequestURI = ""
	if t.typ == pathModified {
		clone.URL.Path = t.rx.ReplaceAllString(clone.URL.Path, t.replacement)
	}
	return *clone
}

// Creates out tee Filter
// If only one parameter is given shadow backend is used as it is specified
// If second and third parameters are also set, then path is modified
func (spec *teeSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	tee := tee{client: &http.Client{}}

	if len(config) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	backend, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	if url, err := url.Parse(backend); err == nil {
		tee.host = url.Host
		tee.scheme = url.Scheme
	} else {
		return nil, err
	}

	if len(config) == 1 {
		tee.typ = asBackend
		return &tee, nil
	}

	//modpath
	if len(config) == 3 {
		expr, ok := config[1].(string)
		if !ok {
			return nil, fmt.Errorf("invalid filter config in %s, expecting regexp and string, got: %v", Name, config)
		}

		replacement, ok := config[2].(string)
		if !ok {
			return nil, fmt.Errorf("invalid filter config in %s, expecting regexp and string, got: %v", Name, config)
		}

		rx, err := regexp.Compile(expr)

		if err != nil {
			return nil, err
		}
		tee.typ = pathModified
		tee.rx = rx
		tee.replacement = replacement

		return &tee, nil
	}

	return nil, filters.ErrInvalidFilterParameters
}

func (spec *teeSpec) Name() string { return Name }
