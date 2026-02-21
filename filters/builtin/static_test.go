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

package builtin

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestStatic(t *testing.T) {
	const testData = "Hello, world!"

	for _, ti := range []struct {
		msg             string
		args            []any
		content         string
		removeFile      bool
		path            string
		expectedStatus  int
		expectedContent string
	}{{
		msg:            "invalid number of args",
		args:           nil,
		content:        testData,
		path:           "/static/static-test",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:            "not string web root",
		args:           []any{3.14, "/tmp"},
		content:        testData,
		path:           "/static/static-test",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:            "not string fs root",
		args:           []any{"/static", 3.14},
		content:        testData,
		path:           "/static/static-test",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:            "web root cannot be clipped",
		args:           []any{"/static", "/tmp"},
		content:        testData,
		path:           "/a",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:            "not found",
		args:           []any{"/static", "/tmp"},
		content:        testData,
		removeFile:     true,
		path:           "/static/static-test",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:             "found",
		args:            []any{"/static", "/tmp"},
		content:         testData,
		path:            "/static/static-test",
		expectedStatus:  http.StatusOK,
		expectedContent: testData,
	}, {
		msg:             "found, empty",
		args:            []any{"/static", "/tmp"},
		content:         "",
		path:            "/static/static-test",
		expectedStatus:  http.StatusOK,
		expectedContent: "",
	}} {
		if ti.removeFile {
			if err := os.Remove("/tmp/static-test"); err != nil && !os.IsNotExist(err) {
				t.Error(ti.msg, err)
				continue
			}
		} else {
			if err := os.WriteFile("/tmp/static-test", []byte(ti.content), os.ModePerm); err != nil {
				t.Error(ti.msg, err)
				continue
			}
		}

		fr := make(filters.Registry)
		fr.Register(NewStatic())
		pr := proxytest.New(fr, &eskip.Route{
			Filters: []*eskip.Filter{{Name: filters.StaticName, Args: ti.args}},
			Shunt:   true})
		defer pr.Close()

		rsp, err := http.Get(pr.URL + ti.path)
		if err != nil {
			t.Error(ti.msg, err)
			continue
		}

		defer rsp.Body.Close()

		if rsp.StatusCode != ti.expectedStatus {
			t.Error(ti.msg, "status code doesn't match", rsp.StatusCode, ti.expectedStatus)
			continue
		}

		if rsp.StatusCode != http.StatusOK {
			continue
		}

		content, err := io.ReadAll(rsp.Body)
		if err != nil {
			t.Error(ti.msg, err)
			continue
		}

		if string(content) != ti.expectedContent {
			t.Error(ti.msg, "content doesn't match", string(content), ti.expectedContent)
		}
	}
}

func TestSameFileMultipleTimes(t *testing.T) {
	const n = 6

	if err := os.WriteFile("/tmp/static-test", []byte("test content"), os.ModePerm); err != nil {
		t.Error(err)
		return
	}

	fr := make(filters.Registry)
	fr.Register(NewStatic())
	pr := proxytest.New(fr, &eskip.Route{
		Filters: []*eskip.Filter{{Name: filters.StaticName, Args: []any{"/static", "/tmp"}}},
		Shunt:   true})
	defer pr.Close()

	for range n {
		rsp, err := http.Get(pr.URL + "/static/static-test")
		if err != nil {
			t.Error(err)
			return
		}

		defer rsp.Body.Close()
		_, err = io.ReadAll(rsp.Body)
		if err != nil {
			t.Error(err)
			return
		}
	}
}

func TestMultipleRanges(t *testing.T) {
	const fcontent = "test content"
	if err := os.WriteFile("/tmp/static-test", []byte(fcontent), os.ModePerm); err != nil {
		t.Error(err)
		return
	}

	fr := make(filters.Registry)
	fr.Register(NewStatic())
	pr := proxytest.New(fr, &eskip.Route{
		Filters: []*eskip.Filter{{Name: filters.StaticName, Args: []any{"/static", "/tmp"}}},
		Shunt:   true})
	defer pr.Close()

	req, err := http.NewRequest("GET", pr.URL+"/static/static-test", nil)
	if err != nil {
		t.Error(err)
		return
	}

	req.Close = true
	req.Header.Set("Range", "bytes=1-3,5-8")

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Error(err)
		return
	}

	defer rsp.Body.Close()
	_, params, err := mime.ParseMediaType(rsp.Header.Get("Content-Type"))
	if err != nil {
		t.Error(err)
		return
	}

	mp := multipart.NewReader(rsp.Body, params["boundary"])
	parts := [][]int{{1, 4}, {5, 9}}
	for {
		p, err := mp.NextPart()
		if err != nil {
			if err != io.EOF {
				t.Error(err)
			}

			break
		}

		partContent, err := io.ReadAll(p)
		if err != nil {
			t.Error(err)
			break
		}

		if string(partContent) != fcontent[parts[0][0]:parts[0][1]] {
			t.Error("failed to receive multiple ranges")
		}

		parts = parts[1:]
	}

	if len(parts) != 0 {
		t.Error("failed to receive all ranges")
	}
}
