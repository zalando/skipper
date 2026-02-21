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
	"net/http"
	"reflect"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestBackendIsProxyFilter(t *testing.T) {
	expectedStateBag := map[string]any{
		filters.BackendIsProxyKey: struct{}{},
	}

	ctx := &filtertest.Context{
		FRequest:  &http.Request{},
		FStateBag: map[string]any{},
	}

	f, _ := NewBackendIsProxy().CreateFilter(nil)
	f.Request(ctx)

	if !reflect.DeepEqual(expectedStateBag, ctx.FStateBag) {
		t.Error("StateBags are not equal", expectedStateBag, ctx.FStateBag)
	}
}
