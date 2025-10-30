package tee

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
)

const (
	// Deprecated, use filters.TeeName instead
	Name = filters.TeeName
	// Deprecated, use filters.TeeName instead
	DeprecatedName = "Tee"
	// Deprecated, use filters.TeenfName instead
	NoFollowName = filters.TeenfName
)

const (
	// ShadowTrafficHeader marks outgoing requests as shadow traffic
	ShadowTrafficHeader = "X-Skipper-Shadow-Traffic"

	defaultTeeTimeout          = time.Second
	defaultMaxIdleConns        = 100
	defaultMaxIdleConnsPerHost = 100
	defaultIdleConnTimeout     = 30 * time.Second
)

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

	// Tracer is the opentracing tracer to use in the client
	Tracer opentracing.Tracer

	// MaxIdleConns defaults to 100
	MaxIdleConns int

	// MaxIdleConnsPerHost defaults to 100
	MaxIdleConnsPerHost int

	// IdleConnTimeout defaults to 30s
	IdleConnTimeout time.Duration
}

type teeType int

const (
	asBackend teeType = iota + 1
	pathModified
)

type teeClient struct {
	mu    sync.Mutex
	store map[string]*net.Client
}

var teeClients *teeClient = &teeClient{
	store: make(map[string]*net.Client),
}

type tee struct {
	client            *net.Client
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

// NewTee returns a new tee filter Spec, whose instances execute the exact same Request against a shadow backend.
// parameters: shadow backend url, optional - the path(as a regexp) to match and the replacement string.
//
// Name: "tee".
func NewTee() filters.Spec {
	return WithOptions(Options{
		Timeout:             defaultTeeTimeout,
		NoFollow:            false,
		MaxIdleConns:        defaultMaxIdleConns,
		MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		IdleConnTimeout:     defaultIdleConnTimeout,
	})
}

// NewTeeDeprecated returns a new tee filter Spec, whose instances execute the exact same Request against a shadow backend.
// parameters: shadow backend url, optional - the path(as a regexp) to match and the replacement string.
//
// This version uses the capitalized version of the filter name and to follow conventions, it is deprecated
// and NewTee() (providing the name "tee") should be used instead.
//
// Name: "Tee".
func NewTeeDeprecated() filters.Spec {
	sp := WithOptions(Options{
		NoFollow:            false,
		Timeout:             defaultTeeTimeout,
		MaxIdleConns:        defaultMaxIdleConns,
		MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		IdleConnTimeout:     defaultIdleConnTimeout,
	})
	ts := sp.(*teeSpec)
	ts.deprecated = true
	return ts
}

// NewTeeNoFollow returns a new tee filter Spec, whose instances execute the exact same Request against a shadow backend.
// It does not follow the redirects from the backend.
// parameters: shadow backend url, optional - the path(as a regexp) to match and the replacement string.
//
// Name: "teenf".
func NewTeeNoFollow() filters.Spec {
	return WithOptions(Options{
		NoFollow:            true,
		Timeout:             defaultTeeTimeout,
		MaxIdleConns:        defaultMaxIdleConns,
		MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		IdleConnTimeout:     defaultIdleConnTimeout,
	})
}

// WithOptions returns a new tee filter Spec, whose instances execute the exact same Request against a shadow backend with given
// options. Available options are nofollow and Timeout for http client. For more available options see Options type.
// parameters: shadow backend url, optional - the path(as a regexp) to match and the replacement string.
func WithOptions(o Options) filters.Spec {
	if o.Timeout == 0 {
		o.Timeout = defaultIdleConnTimeout
	}
	if o.MaxIdleConns == 0 {
		o.MaxIdleConns = defaultMaxIdleConns
	}
	if o.MaxIdleConnsPerHost == 0 {
		o.MaxIdleConnsPerHost = defaultMaxIdleConnsPerHost
	}
	if o.IdleConnTimeout == 0 {
		o.IdleConnTimeout = defaultIdleConnTimeout
	}
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

// We do not touch response at all
func (r *tee) Response(filters.FilterContext) {}

// Request is copied and then modified to adopt changes in new backend
func (r *tee) Request(fc filters.FilterContext) {
	req := fc.Request()

	// omit loops
	if req.Header.Get(ShadowTrafficHeader) != "" {
		s := "Skipper Shadow Traffic Loop detected"
		fc.Serve(&http.Response{
			StatusCode: 465,
			Status:     s,
			Header: http.Header{
				"Content-Type":   []string{http.DetectContentType([]byte(s))},
				"Content-Length": []string{strconv.Itoa(len(s))},
			},
			Body: io.NopCloser(bytes.NewBufferString(s)),
		})
		return
	}

	copyOfRequest, tr, err := cloneRequest(r, req)
	if err != nil {
		fc.Logger().Warnf("tee: error while cloning the tee request %v", err)
		return
	}

	req.Header.Set(ShadowTrafficHeader, "yes")
	req.Body = tr

	go func() {
		defer func() {
			if r.shadowRequestDone != nil {
				r.shadowRequestDone()
			}
		}()

		rsp, err := r.client.Do(copyOfRequest)
		if err != nil {
			fc.Logger().Warnf("tee: error while tee request %v", err)
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
	var checkRedirect func(req *http.Request, via []*http.Request) error
	if spec.options.NoFollow {
		checkRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	if len(config) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	backend, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	u, err := url.Parse(backend)
	if err != nil {
		return nil, err
	}

	var client *net.Client
	teeClients.mu.Lock()
	if cc, ok := teeClients.store[u.Host]; !ok {
		client = net.NewClient(net.Options{
			Timeout:                 spec.options.Timeout,
			TLSHandshakeTimeout:     spec.options.Timeout,
			ResponseHeaderTimeout:   spec.options.Timeout,
			CheckRedirect:           checkRedirect,
			MaxIdleConns:            spec.options.MaxIdleConns,
			MaxIdleConnsPerHost:     spec.options.MaxIdleConnsPerHost,
			IdleConnTimeout:         spec.options.IdleConnTimeout,
			Tracer:                  spec.options.Tracer,
			OpentracingComponentTag: "skipper",
			OpentracingSpanName:     spec.Name(),
		})
		teeClients.store[u.Host] = client
	} else {
		client = cc
	}
	teeClients.mu.Unlock()

	tee := tee{
		client: client,
		host:   u.Host,
		scheme: u.Scheme,
	}

	switch len(config) {
	case 1:
		tee.typ = asBackend
		return &tee, nil
	case 3:
		// modpath
		expr, ok := config[1].(string)
		if !ok {
			return nil, fmt.Errorf("invalid filter config in %s, expecting regexp and string, got: %v", filters.TeeName, config)
		}

		replacement, ok := config[2].(string)
		if !ok {
			return nil, fmt.Errorf("invalid filter config in %s, expecting regexp and string, got: %v", filters.TeeName, config)
		}

		rx, err := regexp.Compile(expr)

		if err != nil {
			return nil, err
		}
		tee.typ = pathModified
		tee.rx = rx
		tee.replacement = replacement

		return &tee, nil
	default:
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (spec *teeSpec) Name() string {
	if spec.deprecated {
		return DeprecatedName
	}
	if spec.options.NoFollow {
		return filters.TeenfName
	}
	return filters.TeeName
}
