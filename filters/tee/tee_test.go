package tee

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fmt"

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

type MyHandler struct {
	name string
	body string
}

func TestTeeHostHeaderChanges(t *testing.T) {
	f, _ := testTeeSpec.CreateFilter(teeArgsAsBackend)
	fc := buildfilterContext()

	rep, _ := f.(*tee)
	modifiedRequest := cloneRequest(rep, fc.Request())
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
	modifiedRequest := cloneRequest(rep, fc.Request())
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
	modifiedRequest := cloneRequest(rep, fc.Request())
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
	modifiedRequest := cloneRequest(rep, fc.Request())
	if modifiedRequest.URL.Path != "/v1/" {
		t.Errorf("Tee Request Path not modified, %v", modifiedRequest.URL.Path)
	}

	originalRequest := fc.Request()
	if originalRequest.URL.Path != "/api/v3" {
		t.Errorf("Incoming Request Scheme modified, %v", originalRequest.URL.Path)
	}
}

func (h *MyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := ioutil.ReadAll(r.Body)
	str := string(b)
	h.body = str
}

func TestTeeEndToEndBody(t *testing.T) {
	shadowHandler := &MyHandler{name: "shadow"}
	shadowServer := httptest.NewServer(shadowHandler)
	shadowUrl := shadowServer.URL
	defer shadowServer.Close()

	originalHandler := &MyHandler{name: "original"}
	originalServer := httptest.NewServer(originalHandler)
	originalUrl := originalServer.URL
	defer originalServer.Close()

	routeStr := fmt.Sprintf(`route1: * -> Tee("%v") -> "%v";`, shadowUrl, originalUrl)

	route, _ := eskip.Parse(routeStr)
	registry := make(filters.Registry)
	registry.Register(NewTee())
	p := proxytest.New(registry, route...)
	defer p.Close()

	testingStr := "TESTEST"
	req, _ := http.NewRequest("GET", p.URL, strings.NewReader(testingStr))
	req.Host = "www.example.org"
	req.Header.Set("X-Test", "true")
	req.Close = true
	rsp, _ := (&http.Client{}).Do(req)
	rsp.Body.Close()
	if shadowHandler.body != testingStr && originalHandler.body != testingStr {
		t.Error("Bodies are not equal")
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
