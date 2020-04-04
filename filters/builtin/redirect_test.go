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
				BackendType: eskip.ShuntBackend,
				Filters: []*eskip.Filter{{
					Name: tii.name,
					Args: []interface{}{float64(ti.code), ti.filterLocation}}}}})
			tl := loggingtest.New()
			rt := routing.New(routing.Options{
				FilterRegistry: MakeRegistry(),
				DataClients:    []routing.DataClient{dc},
				Log:            tl})
			p := proxy.New(proxy.Params{
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
