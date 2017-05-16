package proxy

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/zalando/skipper/routing"
)

type context struct {
	responseWriter        http.ResponseWriter
	request               *http.Request
	response              *http.Response
	route                 *routing.Route
	deprecatedServed      bool
	servedWithResponse    bool // to support the deprecated way independently
	pathParams            map[string]string
	stateBag              map[string]interface{}
	originalRequest       *http.Request
	originalResponse      *http.Response
	outgoingHost          string
	debugFilterPanics     []interface{}
	outgoingDebugRequest  *http.Request
	incomingDebugResponse *http.Response
	startServe            time.Time
}

func defaultBody() io.ReadCloser {
	return ioutil.NopCloser(&bytes.Buffer{})
}

func defaultResponse(r *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       defaultBody(),
		Request:    r,
	}
}

func cloneURL(u *url.URL) *url.URL {
	uc := *u
	return &uc
}

func cloneRequestMetadata(r *http.Request) *http.Request {
	return &http.Request{
		Method:           r.Method,
		URL:              cloneURL(r.URL),
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           cloneHeader(r.Header),
		Trailer:          cloneHeader(r.Trailer),
		Body:             defaultBody(),
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Host:             r.Host,
		RemoteAddr:       r.RemoteAddr,
		RequestURI:       r.RequestURI,
		TLS:              r.TLS,
	}
}

func cloneResponseMetadata(r *http.Response) *http.Response {
	return &http.Response{
		Status:           r.Status,
		StatusCode:       r.StatusCode,
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           cloneHeader(r.Header),
		Trailer:          cloneHeader(r.Trailer),
		Body:             defaultBody(),
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Request:          r.Request,
		TLS:              r.TLS,
	}
}

func newContext(w http.ResponseWriter, r *http.Request, preserveOriginal bool) *context {
	c := &context{
		responseWriter: w,
		request:        r,
		stateBag:       make(map[string]interface{}),
		outgoingHost:   r.Host,
	}

	if preserveOriginal {
		c.originalRequest = cloneRequestMetadata(r)
	}

	return c
}

func (c *context) applyRoute(route *routing.Route, params map[string]string, preserveHost bool) {
	c.route = route
	if preserveHost {
		c.outgoingHost = c.request.Host
	} else {
		c.outgoingHost = route.Host
	}

	c.pathParams = params
}

func (c *context) ensureDefaultResponse() {
	if c.response == nil {
		c.response = defaultResponse(c.request)
		return
	}

	if c.response.Header == nil {
		c.response.Header = make(http.Header)
	}

	if c.response.Body == nil {
		c.response.Body = defaultBody()
	}
}

func (c *context) deprecatedShunted() bool {
	return c.deprecatedServed
}

func (c *context) shunted() bool {
	return c.servedWithResponse
}

func (c *context) setResponse(r *http.Response, preserveOriginal bool) {
	c.response = r
	if preserveOriginal {
		c.originalResponse = cloneResponseMetadata(r)
	}
}

func (c *context) ResponseWriter() http.ResponseWriter { return c.responseWriter }
func (c *context) Request() *http.Request              { return c.request }
func (c *context) Response() *http.Response            { return c.response }
func (c *context) MarkServed()                         { c.deprecatedServed = true }
func (c *context) Served() bool                        { return c.deprecatedServed || c.servedWithResponse }
func (c *context) PathParam(key string) string         { return c.pathParams[key] }
func (c *context) StateBag() map[string]interface{}    { return c.stateBag }
func (c *context) BackendUrl() string                  { return c.route.Backend }
func (c *context) OriginalRequest() *http.Request      { return c.originalRequest }
func (c *context) OriginalResponse() *http.Response    { return c.originalResponse }
func (c *context) OutgoingHost() string                { return c.outgoingHost }
func (c *context) SetOutgoingHost(h string)            { c.outgoingHost = h }

func (c *context) Serve(r *http.Response) {
	r.Request = c.Request()

	if r.Header == nil {
		r.Header = make(http.Header)
	}

	if r.Body == nil {
		r.Body = defaultBody()
	}

	c.servedWithResponse = true
	c.response = r
}
