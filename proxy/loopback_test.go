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

func (f *returnPathParam) Name() string                                       { return "returnParam" }
func (f *returnPathParam) CreateFilter([]interface{}) (filters.Filter, error) { return f, nil }
func (f *returnPathParam) Request(filters.FilterContext)                      {}

func (f *returnPathParam) Response(ctx filters.FilterContext) {
	ctx.Response().Header.Add("X-Path-Param", ctx.PathParam("param"))
}

func (s *setState) Name() string { return "setState" }

func (s *setState) CreateFilter(args []interface{}) (filters.Filter, error) {
	return &setState{args[0].(string), args[1].(string)}, nil
}

func (s *setState) Request(ctx filters.FilterContext) {
	ctx.StateBag()[s.name] = s.value
}

func (s *setState) Response(filters.FilterContext) {}

func (c *returnState) Name() string                                       { return "returnState" }
func (c *returnState) CreateFilter([]interface{}) (filters.Filter, error) { return c, nil }
func (c *returnState) Request(filters.FilterContext)                      {}

func (c *returnState) Response(ctx filters.FilterContext) {
	for k, v := range ctx.StateBag() {
		ctx.Response().Header.Add("X-State-Bag", k+"="+v.(string))
	}
}

func testLoopback(
	t *testing.T,
	routes string,
	params Params,
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

	routes = strings.Replace(routes, "$backend", backend.URL, -1)

	fr := builtin.MakeRegistry()
	fr.Register(&preserveOriginalSpec{})
	fr.Register(&returnPathParam{})
	fr.Register(&setState{})
	fr.Register(&returnState{})

	p, err := newTestProxyWithFiltersAndParams(fr, routes, params)
	if err != nil {
		t.Error(err)
		return
	}

	defer p.close()

	u, err := url.ParseRequestURI("https://www.example.org/test/path")
	if err != nil {
		t.Error(err)
		return
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

	testLoopback(t, routes, Params{}, http.StatusTeapot, http.Header{
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

	testLoopback(t, routes, Params{}, http.StatusOK, http.Header{
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

	testLoopback(t, routes, Params{MaxLoopbacks: 3}, http.StatusInternalServerError, http.Header{
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

	testLoopback(t, routes, Params{}, http.StatusInternalServerError, http.Header{
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

	testLoopback(t, routes, Params{Flags: PreserveOriginal}, http.StatusOK, http.Header{
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

	testLoopback(t, routes, Params{Flags: PreserveHost}, http.StatusOK, http.Header{
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
	testLoopback(t, routes, Params{}, http.StatusFound, http.Header{
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

	testLoopback(t, routes, Params{}, http.StatusFound, http.Header{
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

	testLoopback(t, routes, Params{}, http.StatusOK, http.Header{
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

	testLoopback(t, routes, Params{}, http.StatusOK, http.Header{
		"X-Entry-Route-Done": []string{"true"},
		"X-Loop-Route-Done":  []string{"1", "2"},
		"X-Backend-Done":     []string{"true"},
		"X-State-Bag":        []string{"foo=bar"},
	})
}
