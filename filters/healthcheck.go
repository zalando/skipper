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

package filters

import "net/http"

type healthCheck struct{}

// Creates a new filter Spec, whose instances set the status code of the
// response to 200 OK.
func NewHealthCheck() Spec { return &healthCheck{} }

// "healthcheck"
func (h *healthCheck) Name() string { return "healthcheck" }

func (h *healthCheck) CreateFilter(_ []interface{}) (Filter, error) { return h, nil }
func (h *healthCheck) Request(ctx FilterContext)                    {}
func (h *healthCheck) Response(ctx FilterContext)                   { ctx.Response().StatusCode = http.StatusOK }
