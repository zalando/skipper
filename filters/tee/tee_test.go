package tee

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
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

func buildfilterContext() filters.FilterContext {
	r, _ := http.NewRequest("GET", "http://example.org/api/v3", nil)
	return &filtertest.Context{FRequest: r}
}
