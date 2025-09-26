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

package eskip_test

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func Example() {
	code := `

		// Skipper - Eskip:
		// a routing table document, containing multiple route definitions

		// route definition to a jsx page renderer

		route0:
			PathRegexp(/\.html$/) && HeaderRegexp("Accept", "text/html") ->
			modPath(/\.html$/, ".jsx") ->
			requestHeader("X-Type", "page") ->
			"https://render.example.org";

		route1: Path("/some/path") -> "https://backend-0.example.org"; // a simple route

		// route definition with a shunt (no backend address)
		route2: Path("/some/other/path") -> static("/", "/var/www") -> <shunt>;

		// route definition directing requests to an api endpoint
		route3:
			Method("POST") && Path("/api") ->
			requestHeader("X-Type", "ajax-post") ->
			"https://api.example.org";

		// route definition with a loopback to route2 (no backend address)
		route4: Path("/some/alternative/path") -> setPath("/some/other/path") -> <loopback>;
		`

	routes, err := eskip.Parse(code)
	if err != nil {
		log.Println(err)
		return
	}

	format := "%v: [match] -> [%v filter(s) ->] <%v> \"%v\"\n"
	fmt.Println("Parsed routes:")
	for _, r := range routes {
		fmt.Printf(format, r.Id, len(r.Filters), r.BackendType, r.Backend)
	}

	// output:
	// Parsed routes:
	// route0: [match] -> [2 filter(s) ->] <network> "https://render.example.org"
	// route1: [match] -> [0 filter(s) ->] <network> "https://backend-0.example.org"
	// route2: [match] -> [1 filter(s) ->] <shunt> ""
	// route3: [match] -> [1 filter(s) ->] <network> "https://api.example.org"
	// route4: [match] -> [1 filter(s) ->] <loopback> ""
}

func ExampleFilter() {
	code := `
		Method("GET") -> helloFilter("Hello, world!", 3.14) -> "https://backend.example.org"`

	routes, err := eskip.Parse(code)
	if err != nil {
		log.Println(err)
		return
	}

	f := routes[0].Filters[0]

	fmt.Println("Parsed a route with a filter:")
	fmt.Printf("filter name: %v\n", f.Name)
	fmt.Printf("filter arg 0: %v\n", f.Args[0].(string))
	fmt.Printf("filter arg 1: %v\n", f.Args[1].(float64))

	// output:
	// Parsed a route with a filter:
	// filter name: helloFilter
	// filter arg 0: Hello, world!
	// filter arg 1: 3.14
}

func ExampleNetworkBackend() {
	code := `
		ajaxRouteV3: PathRegexp(/^\/api\/v3\/.*/) -> ajaxHeader("v3") -> "https://api.example.org"`

	routes, err := eskip.Parse(code)
	if err != nil {
		log.Println(err)
		return
	}

	r := routes[0]

	fmt.Println("Parsed a route:")
	fmt.Printf("id: %v\n", r.Id)
	fmt.Printf("match regexp: %s\n", r.PathRegexps[0])
	fmt.Printf("# of filters: %v\n", len(r.Filters))
	fmt.Printf("backend type: %v\n", r.BackendType)
	fmt.Printf("backend address: \"%v\"\n", r.Backend)

	// output:
	// Parsed a route:
	// id: ajaxRouteV3
	// match regexp: ^/api/v3/.*
	// # of filters: 1
	// backend type: network
	// backend address: "https://api.example.org"
}

func ExampleLoopBackend() {
	code := `
		ajaxRouteV3: PathRegexp(/^\/api\/v3\/.*/) -> ajaxHeader("v3") -> <loopback>`

	routes, err := eskip.Parse(code)
	if err != nil {
		log.Println(err)
		return
	}

	r := routes[0]

	fmt.Println("Parsed a route:")
	fmt.Printf("id: %v\n", r.Id)
	fmt.Printf("match regexp: %s\n", r.PathRegexps[0])
	fmt.Printf("# of filters: %v\n", len(r.Filters))
	fmt.Printf("backend type: %v\n", r.BackendType)
	fmt.Printf("backend address: \"%v\"\n", r.Backend)

	// output:
	// Parsed a route:
	// id: ajaxRouteV3
	// match regexp: ^/api/v3/.*
	// # of filters: 1
	// backend type: loopback
	// backend address: ""
}

func ExampleShuntBackend() {
	code := `
		ajaxRouteV3: PathRegexp(/^\/api\/v3\/.*/) -> ajaxHeader("v3") -> <shunt>`

	routes, err := eskip.Parse(code)
	if err != nil {
		log.Println(err)
		return
	}

	r := routes[0]

	fmt.Println("Parsed a route:")
	fmt.Printf("id: %v\n", r.Id)
	fmt.Printf("match regexp: %s\n", r.PathRegexps[0])
	fmt.Printf("# of filters: %v\n", len(r.Filters))
	fmt.Printf("backend type: %v\n", r.BackendType)
	fmt.Printf("backend address: \"%v\"\n", r.Backend)

	// output:
	// Parsed a route:
	// id: ajaxRouteV3
	// match regexp: ^/api/v3/.*
	// # of filters: 1
	// backend type: shunt
	// backend address: ""
}

func ExampleParse() {
	code := `
		PathRegexp(/\.html$/) && Header("Accept", "text/html") ->
		modPath(/\.html$/, ".jsx") ->
		requestHeader("X-Type", "page") ->
		"https://render.example.org"`

	routes, err := eskip.Parse(code)
	if err != nil {
		log.Println(err)
		return
	}

	fmt.Printf("Parsed route with backend: %s\n", routes[0].Backend)

	// output: Parsed route with backend: https://render.example.org
}

func ExampleParseFilters() {
	code := `filter0() -> filter1(3.14, "Hello, world!")`
	filters, err := eskip.ParseFilters(code)
	if err != nil {
		log.Println(err)
		return
	}

	fmt.Println("Parsed a chain of filters:")
	fmt.Printf("filters count: %d\n", len(filters))
	fmt.Printf("first filter: %s\n", filters[0].Name)
	fmt.Printf("second filter: %s\n", filters[1].Name)
	fmt.Printf("second filter, first arg: %g\n", filters[1].Args[0].(float64))
	fmt.Printf("second filter, second arg: %s\n", filters[1].Args[1].(string))

	// output:
	// Parsed a chain of filters:
	// filters count: 2
	// first filter: filter0
	// second filter: filter1
	// second filter, first arg: 3.14
	// second filter, second arg: Hello, world!
}

func ExampleForwardBackend() {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	doc := `r: * -> <forward>;`
	routes := eskip.MustParse(doc)

	spec := builtin.NewStatus()
	fr := make(filters.Registry)
	fr.Register(spec)

	dc := testdataclient.New(routes)
	defer dc.Close()

	proxy := proxytest.WithRoutingOptions(fr, routing.Options{
		DataClients:   []routing.DataClient{dc},
		PreProcessors: []routing.PreProcessor{eskip.ForwardPreProcessor(backend.URL)},
	})
	defer proxy.Close()

	client := proxy.Client()

	rsp, err := client.Get("http://hello.world")
	if err != nil {
		fmt.Printf("Failed to GET hello.world: %v\n", err)
		return
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		fmt.Printf("Failed to GET OK from http://hello.world, got: %v\n", rsp.StatusCode)
		return
	}
	fmt.Println("hello skipper")

	// output:
	// hello skipper
}
