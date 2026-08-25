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
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"log"
)

type customSpec struct{ name string }
type customFilter struct{ prefix string }

func (s *customSpec) Name() string {
	return s.name
}

// a specification can be used to create filter instances with different config
func (s *customSpec) CreateFilter(config []any) (filters.Filter, error) {
	if len(config) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	prefix, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &customFilter{prefix}, nil
}

// a simple filter logging the request URLs
func (f *customFilter) Request(ctx filters.FilterContext) {
	log.Println(f.prefix, ctx.Request().URL)
}

func (f *customFilter) Response(_ filters.FilterContext) {}

func Example() {
	// create registry
	registry := builtin.MakeRegistry()

	// create and register the filter specification
	spec := &customSpec{name: "customFilter"}
	registry.Register(spec)

	// create simple data client, with route entries referencing 'customFilter',
	// and clipping part of the request path:
	dataClient, err := testdataclient.NewDoc(`

		ui: Path("/ui/*page") ->
			customFilter("ui request") ->
			modPath("^/[^/]*", "") ->
			"https://ui.example.org";

		api: Path("/api/*resource") ->
			customFilter("api request") ->
			modPath("^/[^/]*", "") ->
			"https://api.example.org"`)

	if err != nil {
		log.Fatal(err)
	}

	// create routing object:
	rt := routing.New(routing.Options{
		FilterRegistry: registry,
		DataClients:    []routing.DataClient{dataClient}})
	defer rt.Close()

	// create http.Handler:
	p := proxy.New(rt, proxy.OptionsNone)
	defer p.Close()
}
