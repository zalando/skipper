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
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"testing"
)

func TestRequestHeader(t *testing.T) {
	spec := filters.CreateRequestHeader()
	if spec.Name() != "requestHeader" {
		t.Error("invalid name")
	}

	f, err := spec.CreateFilter([]interface{}{"Some-Header", "some-value"})
	if err != nil {
		t.Error(err)
	}

	r, err := http.NewRequest("GET", "test:", nil)
	if err != nil {
		t.Error(err)
	}

	c := &filtertest.Context{FRequest: r}
	f.Request(c)
	if r.Header.Get("Some-Header") != "some-value" {
		t.Error("failed to set request header")
	}
}

func TestRequestHeaderInvalidConfigLength(t *testing.T) {
	spec := filters.CreateRequestHeader()
	_, err := spec.CreateFilter([]interface{}{"Some-Header"})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestRequestHeaderInvalidConfigKey(t *testing.T) {
	spec := filters.CreateRequestHeader()
	_, err := spec.CreateFilter([]interface{}{1, "some-value"})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestRequestHeaderInvalidConfigValue(t *testing.T) {
	spec := filters.CreateRequestHeader()
	_, err := spec.CreateFilter([]interface{}{"Some-Header", 2})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestResponseHeader(t *testing.T) {
	spec := filters.CreateResponseHeader()
	if spec.Name() != "responseHeader" {
		t.Error("invalid name")
	}

	f, err := spec.CreateFilter([]interface{}{"Some-Header", "some-value"})
	if err != nil {
		t.Error(err)
	}

	r := &http.Response{Header: make(http.Header)}
	c := &filtertest.Context{FResponse: r}
	f.Response(c)
	if r.Header.Get("Some-Header") != "some-value" {
		t.Error("failed to set request header")
	}
}

func TestResponseHeaderInvalidConfigLength(t *testing.T) {
	spec := filters.CreateResponseHeader()
	_, err := spec.CreateFilter([]interface{}{"Some-Header"})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestResponseHeaderInvalidConfigKey(t *testing.T) {
	spec := filters.CreateResponseHeader()
	_, err := spec.CreateFilter([]interface{}{1, "some-value"})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestResponseHeaderInvalidConfigValue(t *testing.T) {
	spec := filters.CreateResponseHeader()
	_, err := spec.CreateFilter([]interface{}{"Some-Header", 2})
	if err == nil {
		t.Error("failed to fail")
	}
}
