// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
