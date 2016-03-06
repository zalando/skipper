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
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
)

func TestStatic(t *testing.T) {
	const testData = "Hello, world!"

	for _, ti := range []struct {
		msg             string
		args            []interface{}
		removeFile      bool
		path            string
		expectedStatus  int
		expectedContent string
	}{{
		msg:            "invalid number of args",
		args:           nil,
		path:           "/static/static-test",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:            "not string web root",
		args:           []interface{}{3.14, "/tmp"},
		path:           "/static/static-test",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:            "not string fs root",
		args:           []interface{}{"/static", 3.14},
		path:           "/static/static-test",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:            "web root cannot be clipped",
		args:           []interface{}{"/static", "/tmp"},
		path:           "/a",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:            "not found",
		args:           []interface{}{"/static", "/tmp"},
		removeFile:     true,
		path:           "/static/static-test",
		expectedStatus: http.StatusNotFound,
	}, {
		msg:             "found",
		args:            []interface{}{"/static", "/tmp"},
		path:            "/static/static-test",
		expectedStatus:  http.StatusOK,
		expectedContent: testData,
	}} {
		if ti.removeFile {
			if err := os.Remove("/tmp/static-test"); err != nil && !os.IsNotExist(err) {
				t.Error(ti.msg, err)
				continue
			}
		} else {
			if err := ioutil.WriteFile("/tmp/static-test", []byte(testData), os.ModePerm); err != nil {
				t.Error(ti.msg, err)
				continue
			}
		}

		fr := make(filters.Registry)
		fr.Register(NewStatic())
		pr := proxytest.New(fr, &eskip.Route{
			Filters: []*eskip.Filter{{Name: StaticName, Args: ti.args}},
			Shunt:   true})

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

		content, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			t.Error(ti.msg, err)
			continue
		}

		if string(content) != ti.expectedContent {
			t.Error(ti.msg, "content doesn't match", string(content), ti.expectedContent)
		}
	}
}
