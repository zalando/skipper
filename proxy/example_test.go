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

package proxy_test

import (
	"fmt"
	"github.com/rcrowley/go-metrics"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
)

// custom filter type:
type setEchoHeader struct{}

func (s *setEchoHeader) Name() string                                         { return "setEchoHeader" }
func (s *setEchoHeader) CreateFilter(_ []interface{}) (filters.Filter, error) { return s, nil }
func (f *setEchoHeader) Response(_ filters.FilterContext)                     {}

// the filter copies the path parameter 'echo' to the 'X-Echo' header
func (f *setEchoHeader) Request(ctx filters.FilterContext) {
	ctx.Request().Header.Set("X-Echo", ctx.PathParam("echo"))
}

func Example() {
	// create a target backend server. It will return the value of the 'X-Echo' request header
	// as the response body:
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Header.Get("X-Echo")))
	}))

	defer targetServer.Close()

	// create a filter registry, and register the custom filter:
	filterRegistry := builtin.MakeRegistry()
	filterRegistry.Register(&setEchoHeader{})

	// create a data client with a predefined route, referencing the filter and a path condition
	// containing a wildcard called 'echo':
	routeDoc := fmt.Sprintf(`Path("/return/:echo") -> setEchoHeader() -> "%s"`, targetServer.URL)
	dataClient, err := testdataclient.NewDoc(routeDoc)
	if err != nil {
		log.Fatal(err)
	}

	// create a proxy instance, and start an http server:
	proxy := proxy.New(routing.New(routing.Options{
		FilterRegistry: filterRegistry,
		DataClients:    []routing.DataClient{dataClient}}), proxy.OptionsNone, metrics.NewRegistry())
	router := httptest.NewServer(proxy)
	defer router.Close()

	// make a request to the proxy:
	rsp, err := http.Get(fmt.Sprintf("%s/return/Hello,+world!", router.URL))
	if err != nil {
		log.Fatal(err)
	}

	defer rsp.Body.Close()

	// print out the response:
	if _, err := io.Copy(os.Stdout, rsp.Body); err != nil {
		log.Fatal(err)
	}

	// Output:
	// Hello, world!
}

type priorityRoute struct{}

func (p *priorityRoute) Match(request *http.Request) (*routing.Route, map[string]string) {
	if request.URL.Path != "/disabled-page" {
		return nil, nil
	}

	return &routing.Route{Route: eskip.Route{Shunt: true}}, nil
}

func ExamplePriorityRoute() {
	// create a routing doc forwarding all requests,
	// and load it in a data client:
	routeDoc := `Any() -> "https://www.example.org"`
	dataClient, err := testdataclient.NewDoc(routeDoc)
	if err != nil {
		log.Fatal(err)
	}

	// create a priority route making exceptions:
	pr := &priorityRoute{}

	// create an http.Handler:
	proxy.New(
		routing.New(routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
			DataClients:    []routing.DataClient{dataClient}}),
		proxy.OptionsNone,
		metrics.NewRegistry(),
		pr)
}
