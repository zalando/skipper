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
	"github.com/zalando/skipper/eskip"
	"log"
)

func Example() {
	code := `

        // Skipper - Eskip:
        // routing table document, containing multiple route definitions

        // route definition to a jsx page renderer
        route0:
            PathRegexp(/\.html$/) && HeaderRegexp("Accept", "text/html") ->
            pathRewrite(/\.html$/, ".jsx") ->
            requestHeader("X-Type", "page") ->
            "https://render.example.org";
        
        route1: Path("/some/path") -> "https://backend-0.example.org"; // a simple route

        // route definition with a shunt (no backend address)
        route2: Path("/some/other/path") -> fixPath() -> <shunt>;
        
        // route definition directing requests to an api endpoint
        route3:
            Method("POST") && Path("/api") ->
            requestHeader("X-Type", "ajax-post") ->
            "https://api.example.org"

        `

	routes, err := eskip.Parse(code)
	if err != nil {
		log.Println(err)
		return
	}

	format := "%v: [match] -> [%v filter(s) ->] <%v> \"%v\"\n"
	fmt.Println("Parsed routes:")
	for _, r := range routes {
		fmt.Printf(format, r.Id, len(r.Filters), r.Shunt, r.Backend)
	}

	// output:
	// Parsed routes:
	// route0: [match] -> [2 filter(s) ->] <false> "https://render.example.org"
	// route1: [match] -> [0 filter(s) ->] <false> "https://backend-0.example.org"
	// route2: [match] -> [1 filter(s) ->] <true> ""
	// route3: [match] -> [1 filter(s) ->] <false> "https://api.example.org"
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

func ExampleRoute() {
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
	fmt.Printf("is shunt: %v\n", r.Shunt)
	fmt.Printf("backend address: \"%v\"\n", r.Backend)

	// output:
	// Parsed a route:
	// id: ajaxRouteV3
	// match regexp: ^/api/v3/.*
	// # of filters: 1
	// is shunt: false
	// backend address: "https://api.example.org"
}

func ExampleParse() {
	code := `
        PathRegexp(/\.html$/) && Header("Accept", "text/html") ->
        pathRewrite(/\.html$/, ".jsx") ->
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
