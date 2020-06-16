package tee

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy/proxytest"
)

var (
	testTeeSpec        = NewTee()
	teeArgsAsBackend   = []interface{}{"https://api.example.com"}
	teeArgsWithModPath = []interface{}{"https://api.example.com", ".*", "/v1/"}
)

type myHandler struct {
	t      *testing.T
	name   string
	header http.Header
	body   string
	served chan struct{}
}

func TestTeeHostHeaderChanges(t *testing.T) {
	f, _ := testTeeSpec.CreateFilter(teeArgsAsBackend)
	fc := buildfilterContext()

	rep, _ := f.(*tee)
	modifiedRequest, _, err := cloneRequest(rep, fc.Request())
	if err != nil {
		t.Error(err)
		return
	}

	if modifiedRequest.Host != "api.example.com" {
		t.Error("Tee Request Host not modified")
	}

	originalRequest := fc.Request()
	if originalRequest.Host == "api.example.com" {
		t.Error("Incoming Request Host was modified")
	}
}

func TestTeeSchemeChanges(t *testing.T) {
	f, _ := testTeeSpec.CreateFilter(teeArgsAsBackend)
	fc := buildfilterContext()

	rep, _ := f.(*tee)
	modifiedRequest, _, err := cloneRequest(rep, fc.Request())
	if err != nil {
		t.Error(err)
		return
	}

	if modifiedRequest.URL.Scheme != "https" {
		t.Error("Tee Request Scheme not modified")
	}

	originalRequest := fc.Request()
	if originalRequest.URL.Scheme == "https" {
		t.Error("Incoming Request Scheme was modified")
	}
}

func TestTeeUrlHostChanges(t *testing.T) {
	f, _ := testTeeSpec.CreateFilter(teeArgsAsBackend)
	fc := buildfilterContext()

	rep, _ := f.(*tee)
	modifiedRequest, _, err := cloneRequest(rep, fc.Request())
	if err != nil {
		t.Error(err)
		return
	}

	if modifiedRequest.URL.Host != "api.example.com" {
		t.Error("Tee Request Url Host not modified")
	}

	originalRequest := fc.Request()
	if originalRequest.URL.Host == "api.example.com" {
		t.Error("Incoming Request Url Host was modified")
	}
}

func TestTeeWithPathChanges(t *testing.T) {
	f, _ := testTeeSpec.CreateFilter(teeArgsWithModPath)
	fc := buildfilterContext()

	rep, _ := f.(*tee)
	modifiedRequest, _, err := cloneRequest(rep, fc.Request())
	if err != nil {
		t.Error(err)
		return
	}

	if modifiedRequest.URL.Path != "/v1/" {
		t.Errorf("Tee Request Path not modified, %v", modifiedRequest.URL.Path)
	}

	originalRequest := fc.Request()
	if originalRequest.URL.Path != "/api/v3" {
		t.Errorf("Incoming Request Scheme modified, %v", originalRequest.URL.Path)
	}
}

func newTestHandler(t *testing.T, name string) *myHandler {
	return &myHandler{
		t:      t,
		name:   name,
		served: make(chan struct{}),
	}
}

func (h *myHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.t.Error(err)
	}
	h.header = r.Header
	h.body = string(b)
	close(h.served)
}

func TestTeeEndToEndBody(t *testing.T) {
	shadowHandler := newTestHandler(t, "shadow")
	shadowServer := httptest.NewServer(shadowHandler)
	shadowUrl := shadowServer.URL
	defer shadowServer.Close()

	originalHandler := newTestHandler(t, "original")
	originalServer := httptest.NewServer(originalHandler)
	originalUrl := originalServer.URL
	defer originalServer.Close()

	routeStr := fmt.Sprintf(`route1: * -> tee("%v") -> "%v";`, shadowUrl, originalUrl)

	route, _ := eskip.Parse(routeStr)
	registry := make(filters.Registry)
	registry.Register(NewTee())
	p := proxytest.New(registry, route...)
	defer p.Close()

	testingStr := "TESTEST"
	req, err := http.NewRequest("GET", p.URL, strings.NewReader(testingStr))
	if err != nil {
		t.Error(err)
	}

	req.Host = "www.example.org"
	req.Close = true
	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Error(err)
	}

	<-shadowHandler.served

	rsp.Body.Close()
	if shadowHandler.body != testingStr || originalHandler.body != testingStr {
		t.Error("Bodies are not equal")
	}
}

func TestTeeFollowOrNot(t *testing.T) {
	for _, follow := range []bool{
		true,
		false,
	} {
		followed := make(chan struct{})

		shadowRedirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(followed)
		}))
		defer shadowRedirectServer.Close()

		redirectorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, shadowRedirectServer.URL, http.StatusMovedPermanently)
		}))
		defer redirectorServer.Close()

		var fspec filters.Spec
		if follow {
			fspec = NewTee()
		} else {
			fspec = NewTeeNoFollow()
		}

		f, err := fspec.CreateFilter([]interface{}{redirectorServer.URL})
		if err != nil {
			t.Fatal(err)
		}

		done := make(chan struct{})

		f.(*tee).shadowRequestDone = func() {
			select {
			case <-followed:
			default:
				close(done)
			}
		}

		u, err := url.Parse("http://example.org")
		if err != nil {
			t.Fatal(err)
		}

		ctx := &filtertest.Context{
			FRequest: &http.Request{
				URL: u,
			},
		}

		f.Request(ctx)

		select {
		case <-followed:
			if !follow {
				t.Error()
			}
		case <-done:
			if follow {
				t.Error("did not follow the redirect")
			}
		}
	}
}

func TestTeeHeaders(t *testing.T) {
	shadowHandler := newTestHandler(t, "shadow")
	shadowServer := httptest.NewServer(shadowHandler)
	defer shadowServer.Close()

	originalHandler := newTestHandler(t, "original")
	originalServer := httptest.NewServer(originalHandler)
	defer originalServer.Close()

	routeStr := fmt.Sprintf(`route1: * -> tee("%v") -> "%v";`, shadowServer.URL, originalServer.URL)

	route, _ := eskip.Parse(routeStr)
	registry := make(filters.Registry)
	registry.Register(NewTee())
	p := proxytest.New(registry, route...)
	defer p.Close()

	testHeader := "X-Test"
	testHeaderValue := "test-value"

	req, err := http.NewRequest("GET", p.URL, nil)
	if err != nil {
		t.Error(err)
	}

	req.Host = "www.example.org"
	req.Header.Set(testHeader, testHeaderValue)
	req.Header.Set("Proxy-Authorization", "foo")

	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Error(err)
	}

	rsp.Body.Close()

	<-shadowHandler.served

	if shadowHandler.header.Get(testHeader) != testHeaderValue {
		t.Error("failed to forward the header to the shadow host",
			shadowHandler.header.Get(testHeader), testHeaderValue)
	}

	if shadowHandler.header.Get("Proxy-Authorization") != "" {
		t.Error("failed to ignore hop-by-hop headers")
	}
}

func buildfilterContext() filters.FilterContext {
	r, _ := http.NewRequest("GET", "http://example.org/api/v3", nil)
	return &filtertest.Context{FRequest: r}
}

func TestTeeArgsForFailure(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		args []interface{}
		err  bool
	}{
		{
			"error on zero args",
			[]interface{}{},
			true,
		},
		{
			"error on non string args",
			[]interface{}{1},
			true,
		},

		{
			"error on non parsable urls",
			[]interface{}{"%"},
			true,
		},

		{
			"error with 2 arguments",
			[]interface{}{"http://example.com", "test"},
			true,
		},

		{
			"error on non regexp",
			[]interface{}{"http://example.com", 1, "replacement"},
			true,
		},
		{
			"error on non replacement string",
			[]interface{}{"http://example.com", ".*", 1},
			true,
		},

		{
			"error on too many arguments",
			[]interface{}{"http://example.com", ".*", "/api", 5, 6},
			true,
		},

		{
			"error on non valid regexp(trailing slash)",
			[]interface{}{"http://example.com", `\`, "/api"},
			true,
		},
	} {
		_, err := NewTee().CreateFilter(ti.args)

		if ti.err && err == nil {
			t.Error(ti.msg, "was expecting error")
		}

		if !ti.err && err != nil {
			t.Error(ti.msg, "get unexpected error")
		}

		if err != nil {
			continue
		}

	}
}

func TestName(t *testing.T) {
	for _, ti := range []struct {
		spec filters.Spec
		name string
	}{
		{NewTee(), "tee"},
		{NewTeeDeprecated(), "Tee"},
		{NewTeeNoFollow(), "teenf"},
	} {
		n := ti.spec.Name()
		if n != ti.name {
			t.Errorf("expected name %v, got %v", ti.name, n)
		}
	}
}
