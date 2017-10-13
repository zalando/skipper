package builtin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestRedirect(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		code           int
		filterLocation string
		checkLocation  string
	}{{
		"schema only",
		http.StatusFound,
		"http:",
		"http://incoming.example.org/some/path?foo=1&bar=2",
	}, {
		"schema and host",
		http.StatusFound,
		"http://redirect.example.org",
		"http://redirect.example.org/some/path?foo=1&bar=2",
	}, {
		"schema, host and path",
		http.StatusFound,
		"http://redirect.example.org/some/other/path",
		"http://redirect.example.org/some/other/path?foo=1&bar=2",
	}, {
		"schema, host, path and query",
		http.StatusFound,
		"http://redirect.example.org/some/other/path?newquery=3",
		"http://redirect.example.org/some/other/path?newquery=3",
	}, {
		"host only",
		http.StatusFound,
		"//redirect.example.org",
		"https://redirect.example.org/some/path?foo=1&bar=2",
	}, {
		"host and path",
		http.StatusFound,
		"//redirect.example.org/some/other/path",
		"https://redirect.example.org/some/other/path?foo=1&bar=2",
	}, {
		"host, path and query",
		http.StatusFound,
		"//redirect.example.org/some/other/path?newquery=3",
		"https://redirect.example.org/some/other/path?newquery=3",
	}, {
		"path only",
		http.StatusFound,
		"/some/other/path",
		"https://incoming.example.org/some/other/path?foo=1&bar=2",
	}, {
		"path and query",
		http.StatusFound,
		"/some/other/path?newquery=3",
		"https://incoming.example.org/some/other/path?newquery=3",
	}, {
		"query only",
		http.StatusFound,
		"?newquery=3",
		"https://incoming.example.org/some/path?newquery=3",
	}, {
		"schema and path",
		http.StatusFound,
		"http:///some/other/path",
		"http://incoming.example.org/some/other/path?foo=1&bar=2",
	}, {
		"schema, path and query",
		http.StatusFound,
		"http:///some/other/path?newquery=3",
		"http://incoming.example.org/some/other/path?newquery=3",
	}, {
		"schema and query",
		http.StatusFound,
		"http://?newquery=3",
		"http://incoming.example.org/some/path?newquery=3",
	}, {
		"different code",
		http.StatusMovedPermanently,
		"/some/path",
		"https://incoming.example.org/some/path?foo=1&bar=2",
	}} {
		for _, tii := range []struct {
			msg  string
			name string
		}{{
			"deprecated",
			RedirectName,
		}, {
			"not deprecated",
			RedirectToName,
		}} {
			dc := testdataclient.New([]*eskip.Route{{
				Shunt: true,
				Filters: []*eskip.Filter{{
					Name: tii.name,
					Args: []interface{}{float64(ti.code), ti.filterLocation}}}}})
			tl := loggingtest.New()
			rt := routing.New(routing.Options{
				FilterRegistry: MakeRegistry(),
				DataClients:    []routing.DataClient{dc},
				Log:            tl})
			p := proxy.WithParams(proxy.Params{
				Routing: rt,
			})

			closeAll := func() {
				p.Close()
				rt.Close()
				tl.Close()
			}

			// pick up routing
			if err := tl.WaitFor("route settings applied", time.Second); err != nil {
				t.Error(err)
				closeAll()
				continue
			}

			req := &http.Request{
				URL:  &url.URL{Path: "/some/path", RawQuery: "foo=1&bar=2"},
				Host: "incoming.example.org"}
			w := httptest.NewRecorder()
			p.ServeHTTP(w, req)

			if w.Code != ti.code {
				t.Error(ti.msg, tii.msg, "invalid status code", w.Code)
			}

			if w.Header().Get("Location") != ti.checkLocation {
				t.Error(ti.msg, tii.msg, "invalid location", w.Header().Get("Location"))
			}

			closeAll()
		}
	}
}

func TestRedirectLower(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		code           int
		filterLocation string
		checkLocation  string
	}{{
		"schema, host, path with uppercase",
		http.StatusFound,
		"http://redirect.example.org/SOME/OTHER/PATH",
		"http://redirect.example.org/some/other/path?foo=1&bar=2",
	}, {
		"schema, host, path with mixed case",
		http.StatusFound,
		"http://redirect.example.org/PAth",
		"http://redirect.example.org/path?foo=1&bar=2",
	}, {
		"schema, host, path with lower case",
		http.StatusFound,
		"http://redirect.example.org/path",
		"http://redirect.example.org/path?foo=1&bar=2",
	}, {
		"schema, host, path, query with uppercase",
		http.StatusFound,
		"http://redirect.example.org/PATH?query=1",
		"http://redirect.example.org/path?query=1",
	}} {
		for _, tii := range []struct {
			msg  string
			name string
		}{{
			"lowercase",
			RedirectToLowerName,
		}} {
			dc := testdataclient.New([]*eskip.Route{{
				Shunt: true,
				Filters: []*eskip.Filter{{
					Name: tii.name,
					Args: []interface{}{float64(ti.code), ti.filterLocation}}}}})
			tl := loggingtest.New()
			rt := routing.New(routing.Options{
				FilterRegistry: MakeRegistry(),
				DataClients:    []routing.DataClient{dc},
				Log:            tl})
			p := proxy.WithParams(proxy.Params{
				Routing: rt,
			})

			closeAll := func() {
				p.Close()
				rt.Close()
				tl.Close()
			}

			if err := tl.WaitFor("route settings applied", time.Second); err != nil {
				t.Error(err)
				closeAll()
				continue
			}

			req := &http.Request{
				URL:  &url.URL{Path: "/some/path", RawQuery: "foo=1&bar=2"},
				Host: "incoming.example.org"}
			w := httptest.NewRecorder()
			p.ServeHTTP(w, req)

			if w.Code != ti.code {
				t.Error(ti.msg, tii.msg, "invalid status code", w.Code)
			}

			if w.Header().Get("Location") != ti.checkLocation {
				t.Error(ti.msg, tii.msg, "invalid location", w.Header().Get("Location"))
			}

			closeAll()
		}
	}
}
