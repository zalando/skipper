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

package routing_test

import (
	"fmt"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"log"
	"net/http"
	"time"
)

func Example() {
	// create a data client with a predefined route:
	dataClient, err := testdataclient.NewDoc(
		`Path("/some/path/to/:id") -> requestHeader("X-From", "skipper") -> "https://www.example.org"`)
	if err != nil {
		log.Fatal(err)
	}

	// create a router:
	r := routing.New(routing.Options{
		FilterRegistry:  filters.Defaults(),
		MatchingOptions: routing.IgnoreTrailingSlash,
		DataClients:     []routing.DataClient{dataClient}})

	// let the route data be propagated:
	time.Sleep(36 * time.Millisecond)

	// create a request:
	req, err := http.NewRequest("GET", "https://www.example.com/some/path/to/resource", nil)
	if err != nil {
		log.Fatal(err)
	}

	// match the request with the router:
	route, params := r.Route(req)
	if route == nil {
		log.Fatal("failed to route")
	}

	// verify the matched route and the path params:
	fmt.Println(route.Backend)
	fmt.Println(params["id"])

	// Output:
	// https://www.example.org
	// resource
}
