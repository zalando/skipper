package builtin

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
	"net/http"
	"testing"
)

func TestStatus(t *testing.T) {
	for _, ti := range []struct {
		msg          string
		args         []any
		expectedCode int
	}{{
		msg:          "no arguments",
		args:         nil,
		expectedCode: http.StatusNotFound,
	}, {
		msg:          "too many arguments",
		args:         []any{float64(http.StatusTeapot), "something else"},
		expectedCode: http.StatusNotFound,
	}, {
		msg:          "invalid code argument",
		args:         []any{"418"},
		expectedCode: http.StatusNotFound,
	}, {
		msg:          "set status",
		args:         []any{float64(http.StatusTeapot)},
		expectedCode: http.StatusTeapot,
	}} {
		fr := make(filters.Registry)
		fr.Register(NewStatus())
		pr := proxytest.New(fr, &eskip.Route{
			Filters: []*eskip.Filter{{Name: filters.StatusName, Args: ti.args}},
			Shunt:   true})
		defer pr.Close()

		req, err := http.NewRequest("GET", pr.URL, nil)
		if err != nil {
			t.Error(ti.msg, err)
			continue
		}

		req.Close = true

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Error(ti.msg, err)
			continue
		}

		defer rsp.Body.Close()

		if rsp.StatusCode != ti.expectedCode {
			t.Error(ti.msg, "status code doesn't match", rsp.StatusCode, ti.expectedCode)
			continue
		}
	}
}
