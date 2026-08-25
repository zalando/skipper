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
	"net/url"

	"github.com/zalando/skipper/filters"
)

type spec struct{}

type filter bool

// PreserveHost returns a filter specification whose filter instances are used to override
// the `proxyPreserveHost` behavior for individual routes.
//
// Instances expect one argument, with the possible values: "true" or "false",
// where "true" means to use the Host header from the incoming request, and
// "false" means to use the host from the backend address.
//
// The filter takes no effect in either case if another filter modifies the
// outgoing host header to a value other than the one in the incoming request
// or in the backend address.
func PreserveHost() filters.Spec { return &spec{} }

func (s *spec) Name() string { return filters.PreserveHostName }

func (s *spec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if a, ok := args[0].(string); ok && a == "true" || a == "false" {
		return filter(a == "true"), nil
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (preserve filter) Response(_ filters.FilterContext) {}

func (preserve filter) Request(ctx filters.FilterContext) {
	u, err := url.Parse(ctx.BackendUrl())
	if err != nil {
		ctx.Logger().Errorf("failed to parse backend host in preserveHost filter %v", err)
		return
	}

	if preserve && ctx.OutgoingHost() == u.Host {
		ctx.SetOutgoingHost(ctx.Request().Host)
	} else if !preserve && ctx.OutgoingHost() == ctx.Request().Host {
		ctx.SetOutgoingHost(u.Host)
	}
}
