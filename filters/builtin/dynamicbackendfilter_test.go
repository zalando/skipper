package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"reflect"
	"testing"
)

func TestDynamicBackendFilters(t *testing.T) {
	for _, ti := range []struct {
		msg              string
		spec             filters.Spec
		args             []interface{}
		expectedStateBag map[string]interface{}
		requestHeader    http.Header
		outgoingHost     string
	}{{
		msg:              "set dynamic backend host from header",
		spec:             NewSetDynamicBackendHostFromHeader(),
		args:             []interface{}{"X-Test-Host"},
		expectedStateBag: map[string]interface{}{filters.DynamicBackendHostKey: "example.com"},
		requestHeader:    http.Header{"Host": []string{"old.com"}, "X-Test-Host": []string{"example.com"}},
		outgoingHost:     "example.com",
	}, {
		msg:              "set dynamic backend scheme from header",
		spec:             NewSetDynamicBackendSchemeFromHeader(),
		args:             []interface{}{"X-Test-Scheme"},
		expectedStateBag: map[string]interface{}{filters.DynamicBackendSchemeKey: "https"},
		requestHeader:    http.Header{"Host": []string{"some.com"}, "X-Test-Scheme": []string{"https"}},
	}, {
		msg:              "set dynamic backend url from header",
		spec:             NewSetDynamicBackendUrlFromHeader(),
		args:             []interface{}{"X-Test-Url"},
		expectedStateBag: map[string]interface{}{filters.DynamicBackendURLKey: "https://example.com"},
		requestHeader:    http.Header{"Host": []string{"some.com"}, "X-Test-Url": []string{"https://example.com"}},
		outgoingHost:     "example.com",
	}, {
		msg:              "set dynamic backend host",
		spec:             NewSetDynamicBackendHost(),
		args:             []interface{}{"example.com"},
		expectedStateBag: map[string]interface{}{filters.DynamicBackendHostKey: "example.com"},
		requestHeader:    http.Header{"Host": []string{"some.com"}},
		outgoingHost:     "example.com",
	}, {
		msg:              "set dynamic backend scheme",
		spec:             NewSetDynamicBackendScheme(),
		args:             []interface{}{"https"},
		expectedStateBag: map[string]interface{}{filters.DynamicBackendSchemeKey: "https"},
		requestHeader:    http.Header{"Host": []string{"some.com"}},
	}, {
		msg:              "set dynamic backend url",
		spec:             NewSetDynamicBackendUrl(),
		args:             []interface{}{"https://example.com"},
		expectedStateBag: map[string]interface{}{filters.DynamicBackendURLKey: "https://example.com"},
		requestHeader:    http.Header{"Host": []string{"some.com"}},
		outgoingHost:     "example.com",
	}} {

		f, err := ti.spec.CreateFilter(ti.args)
		if err != nil {
			t.Error(ti.msg, err)
		}

		req, err := http.NewRequest("GET", "example.com", nil)
		if err != nil {
			t.Error(ti.msg, err)
		}

		for n, vs := range ti.requestHeader {
			req.Header[n] = vs
		}

		ctx := &filtertest.Context{FRequest: req, FStateBag: map[string]interface{}{}}
		f.Request(ctx)

		beq := reflect.DeepEqual(ti.expectedStateBag, ctx.FStateBag)
		if !beq {
			t.Error(ti.msg, "<StateBags are not equal>", ti.expectedStateBag, ctx.FStateBag)
		}

		if ti.outgoingHost != "" && ti.outgoingHost != ctx.FOutgoingHost {
			t.Error(ti.msg, "<Out going host is wrong>", ti.outgoingHost, ctx.FOutgoingHost)
		}
	}
}
