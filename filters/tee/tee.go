package tee

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"io"
	// "io/ioutil"
	"net/http"
	"net/url"
	"regexp"
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
	_, copyOfRequest := cloneRequest(r, fc.Request())
	go func() {
		rsp, err := r.client.Do(&copyOfRequest)
		if err != nil {
			log.Warn("error while tee request", err)
		}
		defer rsp.Body.Close()
		// defer pw.Close()
	}()
}

// copies requests changes URL and Host in request.
// If 2nd and 3rd params are given path is also modified by applying regexp
func cloneRequest(rep *tee, req *http.Request) (io.PipeWriter, http.Request) {
	copyOfRequest := new(http.Request)
	*copyOfRequest = *req
	copyUrl := new(url.URL)
	*copyUrl = *req.URL
	copyOfRequest.URL = copyUrl
	copyOfRequest.URL.Host = rep.host
	copyOfRequest.URL.Scheme = rep.scheme
	copyOfRequest.Host = rep.host

	pr, pw := io.Pipe()
	// tr := io.TeeReader(req.Body, pw)
	// copyOfRequest.Body = ioutil.NopCloser(tr)
	tr := &teeTie{req.Body, pw}
	copyOfRequest.Body = pr
	req.Body = tr
	copyOfRequest.RequestURI = ""
	if rep.typ == pathModified {
		copyOfRequest.URL.Path = string(rep.rx.ReplaceAllString(copyOfRequest.URL.Path, rep.replacement))
	}
	return *pw, *copyOfRequest
}

// Creates out tee Filter
// If only one parameter is given shadow backend is used as it is specified
// If second and third parameters are also set, then path is modified
func (spec *teeSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	var teeInit tee = tee{client: &http.Client{}}

	if len(config) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	backend, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	if url, err := url.Parse(backend); err == nil {
		teeInit.host = url.Host
		teeInit.scheme = url.Scheme
	} else {
		return nil, err
	}

	if len(config) == 1 {
		teeInit.typ = asBackend
		return &teeInit, nil
	}

	//modpath
	if len(config) == 3 {
		expr, ok := config[1].(string)
		if !ok {
			return nil, fmt.Errorf("Invalid filter config in %s, expecting regexp and string, got: %v", Name, config)
		}

		replacement, ok := config[2].(string)
		if !ok {
			return nil, fmt.Errorf("invalid filter config in %s, expecting regexp and string, got: %v", Name, config)
		}

		rx, err := regexp.Compile(expr)

		if err != nil {
			return nil, err
		}
		teeInit.typ = pathModified
		teeInit.rx = rx
		teeInit.replacement = replacement

		return &teeInit, nil
	}

	return nil, filters.ErrInvalidFilterParameters
}

func (spec *teeSpec) Name() string { return Name }
