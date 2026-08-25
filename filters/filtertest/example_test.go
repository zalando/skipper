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

package filtertest_test

import (
	"fmt"
	"log"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

type customFilter struct{}

func (f *customFilter) Request(ctx filters.FilterContext) {
	ctx.StateBag()["filter called"] = true
}

func (f *customFilter) Response(ctx filters.FilterContext) {}

func ExampleFilter() {
	// create a test filter and add to the registry:
	fr := builtin.MakeRegistry()
	fr.Register(&filtertest.Filter{FilterName: "testFilter"})

	// create a data client, with a predefined route referencing the filter:
	dc, err := testdataclient.NewDoc(`Path("/some/path/:param") -> testFilter(3.14, "Hello, world!") -> "https://www.example.org"`)
	if err != nil {
		log.Fatal(err)
	}

	// create routing object:
	rt := routing.New(routing.Options{
		DataClients:    []routing.DataClient{dc},
		FilterRegistry: fr})
	defer rt.Close()

	// create an http.Handler:
	p := proxy.New(rt, proxy.OptionsNone)
	defer p.Close()
}

func ExampleContext() {
	// create a filter instance:
	filter := &customFilter{}

	// create a test context:
	ctx := &filtertest.Context{FStateBag: make(map[string]any)}

	// call the request handler method of the filter:
	filter.Request(ctx)
	fmt.Printf("%t", ctx.StateBag()["filter called"].(bool))

	// Output:
	// true
}
