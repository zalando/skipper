package tee

import (
	"bytes"
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy/proxytest"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var (
	testTeeSpec        = NewTee()
	teeArgsAsBackend   = []interface{}{"https://api.example.com"}
	teeArgsWithModPath = []interface{}{"https://api.example.com", ".*", "/v1/"}
)

func TestTeeHostHeaderChanges(t *testing.T) {
	f, _ := testTeeSpec.CreateFilter(teeArgsAsBackend)
	fc := buildfilterContext()

	rep, _ := f.(*tee)
	_, modifiedRequest := cloneRequest(rep, fc.Request())
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
	_, modifiedRequest := cloneRequest(rep, fc.Request())
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
	_, modifiedRequest := cloneRequest(rep, fc.Request())
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
	_, modifiedRequest := cloneRequest(rep, fc.Request())
	if modifiedRequest.URL.Path != "/v1/" {
		t.Errorf("Tee Request Path not modified, %v", modifiedRequest.URL.Path)
	}

	originalRequest := fc.Request()
	if originalRequest.URL.Path != "/api/v3" {
		t.Errorf("Incoming Request Scheme modified, %v", originalRequest.URL.Path)
	}
}

type MyHandler struct {
	name string
	body string
}

func (h *MyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//r.Body.Close()
	b, _ := ioutil.ReadAll(r.Body)
	str := string(b)
	h.body = str
	fmt.Println("Server Running")
	fmt.Println(h.name)
	fmt.Println(str)
	fmt.Println("Served")
}

func TestTeeUrlBodyChanges(t *testing.T) {
	t.Skip()

	f, _ := testTeeSpec.CreateFilter(teeArgsAsBackend)
	str := "Hello World"
	r, _ := http.NewRequest("POST", "http://example.org/api/v3", strings.NewReader(str))
	fc := &filtertest.Context{FRequest: r}

	rep, _ := f.(*tee)
	_, modifiedRequest := cloneRequest(rep, fc.Request())
	r.Body.Close()
	modifiedRequest.Body.Close()
	originalBody, _ := ioutil.ReadAll(r.Body)
	teeBody, _ := ioutil.ReadAll(modifiedRequest.Body)

	// var areEqual bool = reflect.DeepEqual(originalBody, teeBody)
	areEqual := bytes.Equal(originalBody, teeBody)

	if !areEqual {
		t.Error("Bodies are not equal")
	}
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

	routeStr := "route1: * -> Tee(\"" + shadowUrl + "\")" + " -> " + "\"" + originalUrl + "\";"

	route, _ := eskip.Parse(routeStr)
	registery := make(filters.Registry)
	registery.Register(NewTee())
	p := proxytest.New(registery, route...)
	defer p.Close()

	//str := "Hello World"

	req, _ := http.NewRequest("GET", p.URL, strings.NewReader("TESTEST"))
	req.Host = "www.example.org"
	req.Header.Set("X-Test", "true")
	req.Close = true
	rsp, _ := (&http.Client{}).Do(req)
	fmt.Println("Request Done")
	rsp.Body.Close()
	fmt.Println("Response Body Closed")
	if shadowHandler.body != originalHandler.body {
		t.Error("Bodies are not equal")
	}

}

func buildfilterContext() filters.FilterContext {
	r, _ := http.NewRequest("GET", "http://example.org/api/v3", nil)
	return &filtertest.Context{FRequest: r}
}
