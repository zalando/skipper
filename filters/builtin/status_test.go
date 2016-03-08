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
		args         []interface{}
		expectedCode int
	}{{
		msg:          "no arguments",
		args:         nil,
		expectedCode: http.StatusNotFound,
	}, {
		msg:          "too many arguments",
		args:         []interface{}{float64(http.StatusTeapot), "something else"},
		expectedCode: http.StatusNotFound,
	}, {
		msg:          "invalid code argument",
		args:         []interface{}{"418"},
		expectedCode: http.StatusNotFound,
	}, {
		msg:          "set status",
		args:         []interface{}{float64(http.StatusTeapot)},
		expectedCode: http.StatusTeapot,
	}} {
		fr := make(filters.Registry)
		fr.Register(NewStatus())
		pr := proxytest.New(fr, &eskip.Route{
			Filters: []*eskip.Filter{{Name: StatusName, Args: ti.args}},
			Shunt:   true})

		rsp, err := http.Get(pr.URL)
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
