package apimonitoring

import (
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"net/http"
)

func newTestFilterContext(
	path string,
	method string,
) filters.FilterContext {
	return &testFilterContext{
		request: &http.Request{
			RequestURI: path,
			Method: method,
		},
	}
}

type testFilterContext struct {
	request *http.Request
}

var _ filters.FilterContext = new(testFilterContext)

func (*testFilterContext) ResponseWriter() http.ResponseWriter {
	panic("implement me")
}

func (c *testFilterContext) Request() *http.Request {
	return c.request
}

func (*testFilterContext) Response() *http.Response {
	panic("implement me")
}

func (*testFilterContext) OriginalRequest() *http.Request {
	panic("implement me")
}

func (*testFilterContext) OriginalResponse() *http.Response {
	panic("implement me")
}

func (*testFilterContext) Served() bool {
	panic("implement me")
}

func (*testFilterContext) MarkServed() {
	panic("implement me")
}

func (*testFilterContext) Serve(*http.Response) {
	panic("implement me")
}

func (*testFilterContext) PathParam(string) string {
	panic("implement me")
}

func (*testFilterContext) StateBag() map[string]interface{} {
	panic("implement me")
}

func (*testFilterContext) BackendUrl() string {
	panic("implement me")
}

func (*testFilterContext) OutgoingHost() string {
	panic("implement me")
}

func (*testFilterContext) SetOutgoingHost(string) {
	panic("implement me")
}

func (*testFilterContext) Metrics() filters.Metrics {
	panic("implement me")
}

func (*testFilterContext) Tracer() opentracing.Tracer {
	panic("implement me")
}
