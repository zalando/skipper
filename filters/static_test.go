package filters_test

import (
	"bytes"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestStatic(t *testing.T) {
	d := []byte("some data")
	err := ioutil.WriteFile("/tmp/static-test", d, os.ModePerm)
	if err != nil {
		t.Error("failed to create test file")
	}

	s := &filters.Static{}
	f, err := s.CreateFilter([]interface{}{"/static", "/tmp"})
	if err != nil {
		t.Error("failed to create filter")
	}

	fc := &filtertest.Context{
		FResponseWriter: httptest.NewRecorder(),
		FRequest:        &http.Request{URL: &url.URL{Path: "/static/static-test"}}}
	f.Response(fc)

	b, err := ioutil.ReadAll(fc.FResponseWriter.(*httptest.ResponseRecorder).Body)
	if err != nil {
		t.Error("failed to verify response")
	}

	if !bytes.Equal(b, d) {
		t.Error("failed to write response", string(b))
	}
}
