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
	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"net/url"
)

type spec struct{}

type filter bool

// Returns a filter specification that is used to set the 'Host' header
// of the proxy request to the one specified by the incoming request.
func PreserveHost() filters.Spec { return &spec{} }

func (s *spec) Name() string { return PreserveHostName }

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
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
		log.Error("failed to parse backend host in preserveHost filter", err)
		return
	}

	if preserve && ctx.OutgoingHost() == u.Host {
		ctx.SetOutgoingHost(ctx.Request().Host)
	} else if !preserve && ctx.OutgoingHost() == ctx.Request().Host {
		ctx.SetOutgoingHost(u.Host)
	}
}
