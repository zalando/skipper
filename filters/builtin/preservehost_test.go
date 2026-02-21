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

func TestCreate(t *testing.T) {
	for _, ti := range []struct {
		msg    string
		args   []any
		filter filter
		err    bool
	}{{
		"0 arguments",
		[]any{},
		false,
		true,
	}, {
		"too many arguments",
		[]any{"true", "false"},
		false,
		true,
	}, {
		"wrong argument",
		[]any{"foo"},
		false,
		true,
	}, {
		"false",
		[]any{"false"},
		false,
		false,
	}, {
		"true",
		[]any{"true"},
		true,
		false,
	}} {
		f, err := PreserveHost().CreateFilter(ti.args)
		if err == nil && ti.err || err != nil && !ti.err {
			t.Error(ti.msg, "failure case", err, ti.err)
		} else if err == nil {
			if f != ti.filter {
				t.Error(ti.msg, "invalid filter created", f, ti.filter)
			}
		}
	}
}

func TestRequest(t *testing.T) {
	for _, ti := range []struct {
		msg             string
		arg             string
		backendUrl      string
		incomingHost    string
		currentOutgoing string
		checkHost       string
	}{{
		"preserve, currently backend",
		"true",
		"https://backend.example.org",
		"www.example.org",
		"backend.example.org",
		"www.example.org",
	}, {
		"preserve, currently incoming",
		"true",
		"https://backend.example.org",
		"www.example.org",
		"www.example.org",
		"www.example.org",
	}, {
		"preserve, currently custom",
		"true",
		"https://backend.example.org",
		"www.example.org",
		"custom.example.org",
		"custom.example.org",
	}, {
		"preserve, currently http1.0 empty",
		"true",
		"https://backend.example.org",
		"www.example.org",
		"",
		"",
	}, {
		"preserve not, currently backend",
		"false",
		"https://backend.example.org",
		"www.example.org",
		"backend.example.org",
		"backend.example.org",
	}, {
		"preserve not, currently incoming",
		"false",
		"https://backend.example.org",
		"www.example.org",
		"www.example.org",
		"backend.example.org",
	}, {
		"preserve not, currently custom",
		"false",
		"https://backend.example.org",
		"www.example.org",
		"custom.example.org",
		"custom.example.org",
	}, {
		"preserve, currently http1.0 empty",
		"false",
		"https://backend.example.org",
		"www.example.org",
		"",
		"",
	}} {
		ctx := &filtertest.Context{
			FRequest:      &http.Request{Host: ti.incomingHost},
			FBackendUrl:   ti.backendUrl,
			FOutgoingHost: ti.currentOutgoing}
		f, _ := PreserveHost().CreateFilter([]any{ti.arg})
		f.Request(ctx)
		if ctx.OutgoingHost() != ti.checkHost {
			t.Error(ti.msg, ctx.OutgoingHost(), ti.checkHost)
		}
	}
}
