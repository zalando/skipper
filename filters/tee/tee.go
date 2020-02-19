package tee

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	Name           = "tee"
	DeprecatedName = "Tee"
	NoFollowName   = "teenf"
)

const defaultTeeTimeout = time.Second

type teeSpec struct {
	deprecated bool
	options    Options
}

// Options for tee filter.
type Options struct {
	// NoFollow specifies whether tee should follow redirects or not.
	// If NoFollow is true, it won't follow, otherwise it will.
	NoFollow bool

	// Timeout specifies a time limit for requests made by tee filter.
	Timeout time.Duration
}

type teeType int

const (
	asBackend teeType = iota + 1
	pathModified
)

type tee struct {
	client            *http.Client
	typ               teeType
	host              string
	scheme            string
	rx                *regexp.Regexp
	replacement       string
	shadowRequestDone func() // test hook
}

type teeTie struct {
	r io.Reader
	w *io.PipeWriter
}

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
// and
// https://golang.org/src/net/http/httputil/reverseproxy.go
var hopHeaders = []string{
	"Connection",
	"Proxy-Connection", // non-standard but still sent by libcurl and rejected by e.g. google
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",      // canonicalized version of "TE"
	"Trailer", // not Trailers per URL above; http://www.rfc-editor.org/errata_search.php?eid=4522
	"Transfer-Encoding",
	"Upgrade",
}

// Returns a new tee filter Spec, whose instances execute the exact same Request against a shadow backend.
// parameters: shadow backend url, optional - the path(as a regexp) to match and the replacement string.
//
// Name: "tee".
func NewTee() filters.Spec {
	return WithOptions(Options{
		Timeout:  defaultTeeTimeout,
		NoFollow: false,
	})
}

// Returns a new tee filter Spec, whose instances execute the exact same Request against a shadow backend.
// parameters: shadow backend url, optional - the path(as a regexp) to match and the replacement string.
//
// This version uses the capitalized version of the filter name and to follow conventions, it is deprecated
// and NewTee() (providing the name "tee") should be used instead.
//
// Name: "Tee".
func NewTeeDeprecated() filters.Spec {
	return &teeSpec{deprecated: true, options: Options{
		NoFollow: false,
		Timeout:  defaultTeeTimeout,
	}}
}

// Returns a new tee filter Spec, whose instances execute the exact same Request against a shadow backend.
// It does not follow the redirects from the backend.
// parameters: shadow backend url, optional - the path(as a regexp) to match and the replacement string.
//
// Name: "teenf".
func NewTeeNoFollow() filters.Spec {
	return WithOptions(Options{NoFollow: true, Timeout: defaultTeeTimeout})
}

// Returns a new tee filter Spec, whose instances execute the exact same Request against a shadow backend with given
// options. Available options are nofollow and Timeout for http client.
// parameters: shadow backend url, optional - the path(as a regexp) to match and the replacement string.
//
func WithOptions(o Options) filters.Spec {
	return &teeSpec{options: o}
}

func (tt *teeTie) Read(b []byte) (int, error) {
	n, err := tt.r.Read(b)

	if err != nil && err != io.EOF {
		tt.w.CloseWithError(err)
		return n, err
	}

	if n > 0 {
		if _, werr := tt.w.Write(b[:n]); werr != nil {
			log.Error("tee: error while tee request", werr)
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
	req := fc.Request()
	copyOfRequest, tr, err := cloneRequest(r, req)
	if err != nil {
		log.Warn("tee: error while cloning the tee request", err)
		return
	}

	req.Body = tr

	go func() {
		defer func() {
			if r.shadowRequestDone != nil {
				r.shadowRequestDone()
			}
		}()

		rsp, err := r.client.Do(copyOfRequest)

		// Prevent to leak goroutines when request body is never read.
		if fc.Shunted() {
			defer copyOfRequest.Body.Close()
		}

		if err != nil {
			log.Warn("tee: error while tee request", err)
			return
		}

		rsp.Body.Close()
	}()
}

// copies requests changes URL and Host in request.
// If 2nd and 3rd params are given path is also modified by applying regexp
// Returns the cloned request and the tee body to be used on the main request.
func cloneRequest(t *tee, req *http.Request) (*http.Request, io.ReadCloser, error) {
	u := new(url.URL)
	*u = *req.URL
	u.Host = t.host
	u.Scheme = t.scheme
	if t.typ == pathModified {
		u.Path = t.rx.ReplaceAllString(u.Path, t.replacement)
	}

	h := make(http.Header)
	for k, v := range req.Header {
		h[k] = v
	}

	for _, k := range hopHeaders {
		h.Del(k)
	}

	var teeBody io.ReadCloser
	mainBody := req.Body

	// see proxy.go:231
	if req.ContentLength != 0 {
		pr, pw := io.Pipe()
		teeBody = pr
		mainBody = &teeTie{mainBody, pw}
	}

	clone, err := http.NewRequest(req.Method, u.String(), teeBody)
	if err != nil {
		return nil, nil, err
	}

	clone.Header = h
	clone.Host = t.host
	clone.ContentLength = req.ContentLength

	return clone, mainBody, nil
}

// Creates out tee Filter
// If only one parameter is given shadow backend is used as it is specified
// If second and third parameters are also set, then path is modified
func (spec *teeSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	client := &http.Client{Timeout: spec.options.Timeout}

	if spec.options.NoFollow {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	tee := tee{client: client}

	if len(config) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	backend, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	if u, err := url.Parse(backend); err == nil {
		tee.host = u.Host
		tee.scheme = u.Scheme
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

func (spec *teeSpec) Name() string {
	if spec.deprecated {
		return DeprecatedName
	}
	if spec.options.NoFollow {
		return NoFollowName
	}
	return Name
}
