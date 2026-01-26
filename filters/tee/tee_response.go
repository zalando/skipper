package tee

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
)

var teeResponseClients *teeClient = &teeClient{
	store: make(map[string]*net.Client),
}

type (
	teeResponseSpec struct {
		options Options
	}

	teeResponse struct {
		client *net.Client
		host   string
		scheme string
	}
)

// NewTeeResponse returns a new teeResponse filter Spec, whose instances create a Request against a shadow backend with the response body streamed to the client.
// parameters: shadow backend url
//
// Name: "teeResponse".
func NewTeeResponse(opt Options) filters.Spec {
	spec := &teeResponseSpec{options: Options{
		Timeout:             defaultTeeTimeout,
		MaxIdleConns:        defaultMaxIdleConns,
		MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		IdleConnTimeout:     defaultIdleConnTimeout,
	}}
	if opt.Timeout != 0 {
		spec.options.Timeout = opt.Timeout
	}
	if opt.IdleConnTimeout != 0 {
		spec.options.IdleConnTimeout = opt.IdleConnTimeout
	}
	if opt.MaxIdleConns != 0 {
		spec.options.MaxIdleConns = opt.MaxIdleConns
	}
	if opt.MaxIdleConnsPerHost != 0 {
		spec.options.MaxIdleConnsPerHost = opt.MaxIdleConnsPerHost
	}

	return spec
}

func (spec *teeResponseSpec) Name() string {
	return filters.TeeResponseName
}

// CreateFilter creates out teeResponse Filter
// If only one parameter is given shadow backend is used as it is specified
func (spec *teeResponseSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) != 1 {
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
	teeResponseClients.mu.Lock()
	if cc, ok := teeResponseClients.store[u.Host]; !ok {
		client = net.NewClient(net.Options{
			Timeout:                 spec.options.Timeout,
			TLSHandshakeTimeout:     spec.options.Timeout,
			ResponseHeaderTimeout:   spec.options.Timeout,
			MaxIdleConns:            spec.options.MaxIdleConns,
			MaxIdleConnsPerHost:     spec.options.MaxIdleConnsPerHost,
			IdleConnTimeout:         spec.options.IdleConnTimeout,
			Tracer:                  spec.options.Tracer,
			OpentracingComponentTag: "skipper",
			OpentracingSpanName:     spec.Name(),
		})
		teeResponseClients.store[u.Host] = client
	} else {
		client = cc
	}
	teeResponseClients.mu.Unlock()

	teeResponse := teeResponse{
		client: client,
		host:   u.Host,
		scheme: u.Scheme,
	}

	return &teeResponse, nil
}

func (f *teeResponse) Response(fc filters.FilterContext) {
	if fc.Response().ContentLength == 0 {
		return
	}

	pr, pw := io.Pipe()
	fc.Response().Body = &teeTie{fc.Response().Body, pw}

	go func() {
		req, err := http.NewRequest("POST", fmt.Sprintf("%s://%s", f.scheme, f.host), pr)
		if err != nil {
			logrus.Errorf("Failed to create request: %v", err)
			return
		}

		rsp, err := f.client.Do(req)
		if err != nil {
			fc.Logger().Warnf("tee: error while teeResponse request %v", err)
			return
		}

		if rsp.Body != nil {
			io.Copy(io.Discard, rsp.Body)
			rsp.Body.Close()
		}
	}()
}

func (*teeResponse) Request(filters.FilterContext) {}
