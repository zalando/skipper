package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
)

type (
	// filter to return all path parameters as response headers for assertions
	returnPathParam struct{}

	// filter to set a value in the state bag to pass around
	setState struct{ name, value string }

	// filter to return all state bag values as response headers for assertions
	returnState struct{}
)

func (f *returnPathParam) Name() string                               { return "returnParam" }
func (f *returnPathParam) CreateFilter([]any) (filters.Filter, error) { return f, nil }
func (f *returnPathParam) Request(filters.FilterContext)              {}

func (f *returnPathParam) Response(ctx filters.FilterContext) {
	ctx.Response().Header.Add("X-Path-Param", ctx.PathParam("param"))
}

func (s *setState) Name() string { return "setState" }

func (s *setState) CreateFilter(args []any) (filters.Filter, error) {
	return &setState{args[0].(string), args[1].(string)}, nil
}

func (s *setState) Request(ctx filters.FilterContext) {
	ctx.StateBag()[s.name] = s.value
}

func (s *setState) Response(filters.FilterContext) {}

func (c *returnState) Name() string                               { return "returnState" }
func (c *returnState) CreateFilter([]any) (filters.Filter, error) { return c, nil }
func (c *returnState) Request(filters.FilterContext)              {}

func (c *returnState) Response(ctx filters.FilterContext) {
	for k, v := range ctx.StateBag() {
		ctx.Response().Header.Add("X-State-Bag", k+"="+v.(string))
	}
}

func testLoopback(
	t *testing.T,
	routes string,
	params Params,
	input *url.URL,
	expectedStatus int,
	expectedHeader http.Header,
) {
	var backend *httptest.Server
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if params.Flags.PreserveOriginal() {
			if r.Header.Get("X-Test-Preserved") != "test-value" {
				t.Error("failed to preserve original request")
				return
			}

			w.Header().Set("X-Test", "test-value")
		}

		if params.Flags.PreserveHost() && r.Host != "www.example.org" {
			t.Error("failed to preserve host")
		} else if !params.Flags.PreserveHost() {
			u, err := url.Parse(backend.URL)
			if err != nil {
				t.Error(err)
				return
			}

			if r.Host != u.Host {
				t.Error("failed to set host")
				return
			}
		}

		w.Header().Set("X-Backend-Done", "true")
	}))
	defer backend.Close()

	routes = strings.ReplaceAll(routes, "$backend", backend.URL)

	fr := builtin.MakeRegistry()
	fr.Register(&preserveOriginalSpec{})
	fr.Register(&returnPathParam{})
	fr.Register(&setState{})
	fr.Register(&returnState{})
	fr.Register(builtin.NewLoopbackIfStatus())

	p, err := newTestProxyWithFiltersAndParams(fr, routes, params, nil)
	if err != nil {
		t.Error(err)
		return
	}

	defer p.close()

	var u *url.URL
	if input == nil {
		u, err = url.ParseRequestURI("https://www.example.org/test/path")
		if err != nil {
			t.Error(err)
			return
		}
	} else {
		u = input
	}

	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: http.Header{
			"X-Test": []string{"test-value"},
			"Host":   []string{"www.example.org"},
		},
		Host: "www.example.org",
	}

	w := httptest.NewRecorder()

	p.proxy.ServeHTTP(w, r)

	if w.Code != expectedStatus {
		t.Error("failed to set status", w.Code, expectedStatus)
		return
	}

	for k, v := range expectedHeader {
		rv := w.Header()[k]
		if len(rv) != len(v) {
			t.Error("unexpected headers", k, len(rv), len(v))
			return
		}

		for _, vi := range v {
			var found bool
			for i, rvi := range rv {
				if rvi == vi {
					rv = append(rv[:i], rv[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				t.Error("expected header not found", k, vi)
				return
			}
		}
	}

	if params.Flags.PreserveOriginal() && w.Header().Get("X-Test-Preserved") != "test-value" {
		t.Error("failed to preserve original response")
	}
}

func TestLoopbackShunt(t *testing.T) {
	routes := `
		entry: *
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> setRequestHeader("X-Loop-Route", "1")
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> setRequestHeader("X-Loop-Route", "2")
			-> <loopback>;

		loopRoute2: Header("X-Loop-Route", "2")
			-> status(418)
			-> appendResponseHeader("X-Loop-Route-Done", "2")
			-> <shunt>;
	`

	testLoopback(t, routes, Params{}, nil, http.StatusTeapot, http.Header{
		"X-Entry-Route-Done": []string{"true"},
		"X-Loop-Route-Done":  []string{"1", "2"},
	})
}

func TestLoopbackWithBackend(t *testing.T) {
	routes := `
		entry: *
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> setRequestHeader("X-Loop-Route", "1")
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> setRequestHeader("X-Loop-Route", "2")
			-> <loopback>;

		loopRoute2: Header("X-Loop-Route", "2")
			-> appendResponseHeader("X-Loop-Route-Done", "2")
			-> "$backend";
	`

	testLoopback(t, routes, Params{}, nil, http.StatusOK, http.Header{
		"X-Entry-Route-Done": []string{"true"},
		"X-Loop-Route-Done":  []string{"1", "2"},
		"X-Backend-Done":     []string{"true"},
	})
}

func TestLoopbackReachLimit(t *testing.T) {
	routes := `
		entry: *
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> setRequestHeader("X-Loop-Route", "1")
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> <loopback>;
	`

	testLoopback(t, routes, Params{MaxLoopbacks: 3}, nil, http.StatusInternalServerError, http.Header{
		"X-Entry-Route-Done": nil,
		"X-Loop-Route-Done":  nil,
	})
}

func TestLoopbackReachDefaultLimit(t *testing.T) {
	routes := `
		entry: *
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> setRequestHeader("X-Loop-Route", "1")
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> <loopback>;
	`

	testLoopback(t, routes, Params{}, nil, http.StatusInternalServerError, http.Header{
		"X-Entry-Route-Done": nil,
		"X-Loop-Route-Done":  nil,
	})
}

func TestLoopbackPreserveOriginalRequest(t *testing.T) {
	routes := `
		entry: *
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> setRequestHeader("X-Loop-Route", "1")
			-> preserveOriginal()
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> setRequestHeader("X-Loop-Route", "2")
			-> <loopback>;

		loopRoute2: Header("X-Loop-Route", "2")
			-> appendResponseHeader("X-Loop-Route-Done", "2")
			-> "$backend";
	`

	testLoopback(t, routes, Params{Flags: PreserveOriginal}, nil, http.StatusOK, http.Header{
		"X-Entry-Route-Done": []string{"true"},
		"X-Loop-Route-Done":  []string{"1", "2"},
	})
}

func TestLoopbackPreserveHost(t *testing.T) {
	routes := `
		entry: *
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> setRequestHeader("X-Loop-Route", "1")
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> setRequestHeader("X-Loop-Route", "2")
			-> <loopback>;

		loopRoute2: Header("X-Loop-Route", "2")
			-> appendResponseHeader("X-Loop-Route-Done", "2")
			-> "$backend";
	`

	testLoopback(t, routes, Params{Flags: PreserveHost}, nil, http.StatusOK, http.Header{
		"X-Entry-Route-Done": []string{"true"},
		"X-Loop-Route-Done":  []string{"1", "2"},
	})
}

func TestLoopbackDeprecatedFilterShunt(t *testing.T) {
	routes := `
		entry: *
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> setRequestHeader("X-Loop-Route", "1")
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> redirect(302, "/test/path")
			-> setRequestHeader("X-Loop-Route", "2")
			-> <loopback>;

		loopRoute2: Header("X-Loop-Route", "2")
			-> appendResponseHeader("X-Loop-Route-Done", "2")
			-> "$backend";
	`

	// NOTE: the deprecated filter shunting executed the remaining filters, preserving here this wrong
	// behavior to avoid making unrelated changes
	testLoopback(t, routes, Params{}, nil, http.StatusFound, http.Header{
		"X-Entry-Route-Done":  []string{"true"},
		"X-Loop-Route-Done":   []string{"1", "2"},
		"X-Loop-Backend-Done": nil,
	})
}

func TestLoopbackFilterShunt(t *testing.T) {
	routes := `
		entry: *
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> setRequestHeader("X-Loop-Route", "1")
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> redirectTo(302, "/redirect/path")
			-> setRequestHeader("X-Loop-Route", "2")
			-> <loopback>;

		loopRoute2: Header("X-Loop-Route", "2")
			-> appendResponseHeader("X-Loop-Route-Done", "2")
			-> "$backend";
	`

	testLoopback(t, routes, Params{}, nil, http.StatusFound, http.Header{
		"X-Entry-Route-Done":  []string{"true"},
		"X-Loop-Route-Done":   []string{"1"},
		"X-Loop-Backend-Done": nil,
	})
}

func TestLoopbackPathParams(t *testing.T) {
	routes := `
		entry: Path("/:param/path")
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> returnParam() // should add "test"
			-> setRequestHeader("X-Loop-Route", "1")
			-> setPath("/")
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> setRequestHeader("X-Loop-Route", "2")
			-> setPath("/loop-test/path")
			-> <loopback>;

		loopRoute2: Path("/:param/path") && Header("X-Loop-Route", "2")
			-> appendResponseHeader("X-Loop-Route-Done", "2")
			-> returnParam() // should add "loop-test"
			-> "$backend";
	`

	testLoopback(t, routes, Params{}, nil, http.StatusOK, http.Header{
		"X-Entry-Route-Done": []string{"true"},
		"X-Loop-Route-Done":  []string{"1", "2"},
		"X-Backend-Done":     []string{"true"},
		"X-Path-Param":       []string{"test", "loop-test"},
	})
}

func TestLoopbackStatebag(t *testing.T) {
	routes := `
		entry: *
			-> appendResponseHeader("X-Entry-Route-Done", "true")
			-> setRequestHeader("X-Loop-Route", "1")
			-> setState("foo", "bar")
			-> <loopback>;

		loopRoute1: Header("X-Loop-Route", "1")
			-> appendResponseHeader("X-Loop-Route-Done", "1")
			-> setRequestHeader("X-Loop-Route", "2")
			-> <loopback>;

		loopRoute2: Header("X-Loop-Route", "2")
			-> appendResponseHeader("X-Loop-Route-Done", "2")
			-> returnState()
			-> "$backend";
	`

	testLoopback(t, routes, Params{}, nil, http.StatusOK, http.Header{
		"X-Entry-Route-Done": []string{"true"},
		"X-Loop-Route-Done":  []string{"1", "2"},
		"X-Backend-Done":     []string{"true"},
		"X-State-Bag":        []string{"foo=bar"},
	})
}

func TestLoopbackWithResponse(t *testing.T) {
	// create registry
	registry := builtin.MakeRegistry()

	// create and register the filter specification
	spec := builtin.NewLoopbackIfStatus()
	registry.Register(spec)

	routes := `
INTERNAL_REDIRECT: Path("/internal-redirect/") -> loopbackIfStatus(418, "/tea-pot")-> status(418)  -> <shunt>;
NO_RESULTS: Path("/tea-pot") -> status(200) -> inlineContent("I'm a teapot, not a search engine", "text/plain'") -> <shunt>;
`

	for _, ti := range []struct {
		msg            string
		input          *url.URL
		expectedStatus int
	}{
		{
			msg:            "request to internal redirect",
			input:          getUrl("/internal-redirect/"),
			expectedStatus: 200,
		},
		{
			msg:            "request to tea-pot",
			input:          getUrl("/tea-pot"),
			expectedStatus: 200,
		},
		{
			msg:            "request to non-existing path",
			input:          getUrl("/non-existing"),
			expectedStatus: 404,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {

			testLoopback(t, routes, Params{}, ti.input, ti.expectedStatus, http.Header{})

		})
	}
}

func getUrl(path string) *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   "www.example.org",
		Path:   path,
	}
}
