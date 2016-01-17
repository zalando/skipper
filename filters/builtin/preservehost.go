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

import "github.com/zalando/skipper/filters"

type preserveHost string

// Returns a filter specification that is used to set the 'Host' header
// of the proxy request to the one specified by the incoming request.
func PreserveHost() filters.Spec { return preserveHost("preserveHost") }

func (s preserveHost) Name() string                                         { return string(s) }
func (s preserveHost) CreateFilter(_ []interface{}) (filters.Filter, error) { return s, nil }
func (s preserveHost) Response(_ filters.FilterContext)                     {}

func (s preserveHost) Request(ctx filters.FilterContext) {
	rhost := ctx.Request().Host
	if rhost != "" {
		ctx.Request().Header.Set("Host", rhost)
	}
}
