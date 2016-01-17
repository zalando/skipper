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
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"testing"
)

func TestPreserveHost(t *testing.T) {
	for _, ti := range []struct{ msg, host string }{{
		"preserve host",
		"www.example.org",
	}, {
		"http 1.0", // without officially supporting
		"",
	}} {
		ctx := &filtertest.Context{
			FRequest: &http.Request{
				Host:   ti.host,
				Header: make(http.Header)}}
		f, _ := PreserveHost().CreateFilter(nil)
		f.Request(ctx)
		if ctx.Request().Header.Get("Host") != ti.host {
			t.Error("host preserve failed", ctx.Request().Header.Get("Host"), ti.host)
		}
	}
}
