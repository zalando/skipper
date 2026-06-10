package routesrv

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters"
)

// passFilter is a filter that does nothing (allows the request through).
type passFilter struct{}

func (passFilter) Request(filters.FilterContext)  {}
func (passFilter) Response(filters.FilterContext) {}

// rejectFilter short-circuits with the given HTTP status code.
type rejectFilter struct{ code int }

func (f rejectFilter) Request(ctx filters.FilterContext) {
	ctx.Serve(&http.Response{StatusCode: f.code, Header: make(http.Header)})
}
func (rejectFilter) Response(filters.FilterContext) {}

// headerCheckFilter reads a request header and stores it in the state bag.
type headerCheckFilter struct{ key string }

func (f headerCheckFilter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[f.key] = ctx.Request().Header.Get(f.key)
}
func (headerCheckFilter) Response(filters.FilterContext) {}

// stateBagReadFilter reads a value from the state bag set by a previous filter.
type stateBagReadFilter struct {
	key   string
	found *bool
}

func (f stateBagReadFilter) Request(ctx filters.FilterContext) {
	_, *f.found = ctx.StateBag()[f.key]
}
func (stateBagReadFilter) Response(filters.FilterContext) {}

// callCountFilter counts how many times Request() is called.
type callCountFilter struct{ count *int }

func (f callCountFilter) Request(filters.FilterContext) { *f.count++ }
func (callCountFilter) Response(filters.FilterContext)  {}

// okHandler responds with 200 and a fixed body.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestWithFilters_NoFilters_PassesThrough(t *testing.T) {
	h := withFilters(okHandler, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWithFilters_EmptySlice_PassesThrough(t *testing.T) {
	h := withFilters(okHandler, []filters.Filter{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWithFilters_PassFilter_HandlerReached(t *testing.T) {
	count := 0
	countHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(http.StatusOK)
	})
	h := withFilters(countHandler, []filters.Filter{passFilter{}})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, count, "handler must be called exactly once")
}

func TestWithFilters_TwoPassFilters_BothCalledAndHandlerReached(t *testing.T) {
	filterCalls := 0
	handlerCalled := false

	counter := callCountFilter{count: &filterCalls}
	countHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	h := withFilters(countHandler, []filters.Filter{counter, counter})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, 2, filterCalls, "both filters must be called")
	assert.True(t, handlerCalled, "handler must be called")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWithFilters_StateBagSharedAcrossFilters(t *testing.T) {
	const key = "X-Token"
	found := false

	h := withFilters(okHandler, []filters.Filter{
		headerCheckFilter{key: key},
		stateBagReadFilter{key: key, found: &found},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	r.Header.Set(key, "secret")
	h.ServeHTTP(w, r)

	require.True(t, found, "second filter must see the state bag value set by the first")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWithFilters_RejectFilter_HandlerNotCalled(t *testing.T) {
	handlerCalled := false
	countHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	h := withFilters(countHandler, []filters.Filter{rejectFilter{code: http.StatusUnauthorized}})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, handlerCalled, "handler must NOT be called after rejection")
}

func TestWithFilters_FirstRejects_SecondNotCalled(t *testing.T) {
	secondCalled := 0
	counter := callCountFilter{count: &secondCalled}

	h := withFilters(okHandler, []filters.Filter{
		rejectFilter{code: http.StatusForbidden},
		counter,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, 0, secondCalled, "second filter must NOT be called after first rejects")
}

func TestWithFilters_FirstAllowsSecondRejects_HandlerNotCalled(t *testing.T) {
	handlerCalled := false
	countHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	h := withFilters(countHandler, []filters.Filter{
		passFilter{},
		rejectFilter{code: http.StatusForbidden},
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.False(t, handlerCalled, "handler must NOT be called after second filter rejects")
}

func TestWithFilters_RejectSetsResponseHeaders(t *testing.T) {
	h := withFilters(okHandler, []filters.Filter{
		filterFunc(func(ctx filters.FilterContext) {
			ctx.Serve(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{"Www-Authenticate": []string{"Bearer realm=test"}},
			})
		}),
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/routes", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "Bearer realm=test", w.Header().Get("Www-Authenticate"))
}

// filterFunc adapts a plain function to filters.Filter.
type filterFunc func(filters.FilterContext)

func (f filterFunc) Request(ctx filters.FilterContext) { f(ctx) }
func (filterFunc) Response(filters.FilterContext)      {}
