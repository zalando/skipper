package proxy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/zalando/skipper/eskip"
)

func TestDebug(t *testing.T) {
	for _, ti := range []struct {
		msg    string
		in     debugInfo
		expect debugDocument
	}{{
		"empty debug info",
		debugInfo{},
		debugDocument{},
	}, {
		"full doc",
		debugInfo{
			route: &eskip.Route{
				Id:         "testRoute",
				Path:       "/hello",
				Backend:    "https://www.example.org",
				Predicates: []*eskip.Predicate{{Name: "Test", Args: []any{3.14, "hello"}}},
				Filters: []*eskip.Filter{
					{Name: "filter0", Args: []any{float64(3.1415), "argvalue"}},
					{Name: "filter1", Args: []any{float64(-42), `ap"argvalue`}}}},
			incoming: &http.Request{
				Method:     "OPTIONS",
				RequestURI: "/testuri",
				Proto:      "HTTP/1.1",
				Header:     http.Header{"X-Test-Header": []string{"test-header-value"}},
				Host:       "test.example.org",
				RemoteAddr: "::1",
				Body:       io.NopCloser(bytes.NewBufferString("incoming body content"))},
			outgoing: &http.Request{
				Method:     "HEAD",
				RequestURI: "/testuri2",
				Proto:      "HTTP/1.1",
				Header:     http.Header{"X-Test-Header-2": []string{"test-header-value-2"}},
				Host:       "www.example.org",
				Body:       io.NopCloser(bytes.NewBufferString("outgoing body content"))},
			response: &http.Response{
				StatusCode: http.StatusTeapot,
				Header:     http.Header{"X-Test-Response-Header": []string{"test-response-header-value"}},
				Body:       io.NopCloser(bytes.NewBufferString("response body"))}},
		debugDocument{
			RouteId: "testRoute",
			Route: (&eskip.Route{
				Path: "/hello", Backend: "https://www.example.org",
				Predicates: []*eskip.Predicate{{Name: "Test", Args: []any{3.14, "hello"}}},
				Filters: []*eskip.Filter{
					{Name: "filter0", Args: []any{float64(3.1415), "argvalue"}},
					{Name: "filter1", Args: []any{float64(-42), `ap"argvalue`}}}}).String(),
			Predicates: []*eskip.Predicate{{Name: "Test", Args: []any{3.14, "hello"}}},
			Filters: []*eskip.Filter{
				{Name: "filter0", Args: []any{float64(3.1415), "argvalue"}},
				{Name: "filter1", Args: []any{float64(-42), `ap"argvalue`}}},
			Incoming: &debugRequest{
				Method:        "OPTIONS",
				Uri:           "/testuri",
				Proto:         "HTTP/1.1",
				Header:        http.Header{"X-Test-Header": []string{"test-header-value"}},
				Host:          "test.example.org",
				RemoteAddress: "::1"},
			Outgoing: &debugRequest{
				Method: "HEAD",
				Uri:    "/testuri2",
				Proto:  "HTTP/1.1",
				Header: http.Header{"X-Test-Header-2": []string{"test-header-value-2"}},
				Host:   "www.example.org"},
			ResponseMod: &debugResponseMod{
				Status: new(http.StatusTeapot),
				Header: http.Header{"X-Test-Response-Header": []string{"test-response-header-value"}}},
			RequestBody:     "outgoing body content",
			ResponseModBody: "response body"},
	}, {
		"route not found",
		debugInfo{
			incoming: &http.Request{
				Method:     "OPTIONS",
				RequestURI: "/testuri",
				Proto:      "HTTP/1.1",
				Header:     http.Header{"X-Test-Header": []string{"test-header-value"}},
				Host:       "test.example.org",
				RemoteAddr: "::1",
				Body:       io.NopCloser(bytes.NewBufferString("incoming body content"))},
			response: &http.Response{StatusCode: http.StatusNotFound}},
		debugDocument{
			Incoming: &debugRequest{
				Method:        "OPTIONS",
				Uri:           "/testuri",
				Proto:         "HTTP/1.1",
				Header:        http.Header{"X-Test-Header": []string{"test-header-value"}},
				Host:          "test.example.org",
				RemoteAddress: "::1"},
			ResponseMod: &debugResponseMod{
				Status: new(http.StatusNotFound)},
			RequestBody: "incoming body content"},
	}, {
		"incoming body when no outgoing",
		debugInfo{
			incoming: &http.Request{
				Method:     "OPTIONS",
				RequestURI: "/testuri",
				Proto:      "HTTP/1.1",
				Header:     http.Header{"X-Test-Header": []string{"test-header-value"}},
				Host:       "test.example.org",
				RemoteAddr: "::1",
				Body:       io.NopCloser(bytes.NewBufferString("incoming body content"))}},
		debugDocument{
			Incoming: &debugRequest{
				Method:        "OPTIONS",
				Uri:           "/testuri",
				Proto:         "HTTP/1.1",
				Header:        http.Header{"X-Test-Header": []string{"test-header-value"}},
				Host:          "test.example.org",
				RemoteAddress: "::1"},
			RequestBody: "incoming body content"},
	}, {
		"no request body",
		debugInfo{
			incoming: &http.Request{
				Method:     "OPTIONS",
				RequestURI: "/testuri",
				Proto:      "HTTP/1.1",
				Header:     http.Header{"X-Test-Header": []string{"test-header-value"}},
				Host:       "test.example.org",
				RemoteAddr: "::1"}},
		debugDocument{
			Incoming: &debugRequest{
				Method:        "OPTIONS",
				Uri:           "/testuri",
				Proto:         "HTTP/1.1",
				Header:        http.Header{"X-Test-Header": []string{"test-header-value"}},
				Host:          "test.example.org",
				RemoteAddress: "::1"}},
	}, {
		"no response",
		debugInfo{response: &http.Response{Header: http.Header{}}},
		debugDocument{},
	}, {
		"response when status",
		debugInfo{
			response: &http.Response{
				StatusCode: http.StatusTeapot,
				Header:     http.Header{}}},
		debugDocument{ResponseMod: &debugResponseMod{Status: new(http.StatusTeapot)}},
	}, {
		"response when header",
		debugInfo{
			response: &http.Response{
				Header: http.Header{"X-Test-Header": []string{"header-value"}}}},
		debugDocument{
			ResponseMod: &debugResponseMod{
				Header: http.Header{"X-Test-Header": []string{"header-value"}}}},
	}, {
		"no response body",
		debugInfo{
			response: &http.Response{
				StatusCode: http.StatusTeapot}},
		debugDocument{
			ResponseMod: &debugResponseMod{
				Status: new(http.StatusTeapot)}},
	}, {
		"error",
		debugInfo{
			err: errors.New("test error"),
		},
		debugDocument{
			ProxyError: "test error",
		},
	}} {
		compareStrings := func(smsg string, got, expect []string) {
			if len(got) != len(expect) {
				t.Error(ti.msg, smsg, "conversion failed")
				return
			}

			for i, v := range got {
				if v != expect[i] {
					t.Error(ti.msg, "failed to convert filter panics")
				}
			}
		}

		compareHeader := func(got, expect http.Header) {
			if len(got) != len(expect) {
				t.Error(ti.msg, "failed to convert header")
				return
			}

			for k, v := range got {
				compareStrings("header", v, expect[k])
			}
		}

		compareRequest := func(got, expect *debugRequest) {
			if got == nil && expect != nil || got != nil && expect == nil {
				t.Error(ti.msg, "failed to convert incoming request")
				return
			}

			if got == nil {
				return
			}

			if got.Method != expect.Method {
				t.Error(ti.msg, "failed to convert method")
			}

			if got.Uri != expect.Uri {
				t.Error(ti.msg, "failed to convert request uri")
			}

			if got.Proto != expect.Proto {
				t.Error(ti.msg, "failed to convert request proto")
			}

			compareHeader(got.Header, expect.Header)

			if got.Host != expect.Host {
				t.Error(ti.msg, "failed to convert request host")
			}

			if got.RemoteAddress != expect.RemoteAddress {
				t.Error(ti.msg, "failed to convert remote address")
			}
		}

		got := convertDebugInfo(&ti.in)

		if got.RouteId != ti.expect.RouteId {
			t.Error(ti.msg, "failed to convert route id")
		}

		if got.Route != ti.expect.Route {
			t.Error(ti.msg, "failed to convert route")
		}

		compareRequest(got.Incoming, ti.expect.Incoming)
		compareRequest(got.Outgoing, ti.expect.Outgoing)

		if got.ResponseMod == nil && ti.expect.ResponseMod != nil ||
			got.ResponseMod != nil && ti.expect.ResponseMod == nil {
			t.Error(ti.msg, "failed to convert response diff")
			continue
		}

		if got.ResponseMod != nil {
			if got.ResponseMod.Status == nil && ti.expect.ResponseMod.Status != nil ||
				got.ResponseMod.Status != nil && ti.expect.ResponseMod.Status == nil ||
				got.ResponseMod.Status != nil && *got.ResponseMod.Status != *ti.expect.ResponseMod.Status {
				t.Error(ti.msg, "failed to convert response modification")
			}

			compareHeader(got.ResponseMod.Header, ti.expect.ResponseMod.Header)
		}

		if got.RequestBody != ti.expect.RequestBody {
			t.Error(ti.msg, "failed to convert request body")
		}

		if got.ResponseModBody != ti.expect.ResponseModBody {
			t.Error(ti.msg, "failed to convert response mod body")
		}

		if got.ProxyError != ti.expect.ProxyError {
			t.Error(ti.msg, "failed to convert proxy error")
		}
	}
}
