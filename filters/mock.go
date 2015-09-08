package filters

import "net/http"

type MockContext struct {
	FResponseWriter http.ResponseWriter
	FRequest        *http.Request
	FResponse       *http.Response
	FServed         bool
}

func (fc *MockContext) ResponseWriter() http.ResponseWriter {
	return fc.FResponseWriter
}

func (fc *MockContext) Request() *http.Request {
	return fc.FRequest
}

func (fc *MockContext) Response() *http.Response {
	return fc.FResponse
}

func (fc *MockContext) MarkServed() {
	fc.FServed = true
}

func (fc *MockContext) IsServed() bool {
	return fc.FServed
}
