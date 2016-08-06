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

package testdataclient_test

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"log"
	"net/http"
	"net/url"
	"time"
)

func Example() {
	// create a data client:
	dataClient := testdataclient.New([]*eskip.Route{
		{Path: "/some/path", Backend: "https://www.example.org"}})

	// create a router:
	r := routing.New(routing.Options{
		DataClients: []routing.DataClient{dataClient}})
	defer r.Close()

	// let the route data be propagated:
	time.Sleep(36 * time.Millisecond)

	// test the router:
	route, _ := r.Route(&http.Request{URL: &url.URL{Path: "/some/path"}})
	if route == nil {
		log.Fatal("failed to route request")
	}

	fmt.Println(route.Backend)

	// Output:
	// https://www.example.org
}

func ExampleNew() {
	testdataclient.New([]*eskip.Route{
		{Path: "/some/path", Backend: "https://www.example.org"}})
}

func ExampleNewDoc() {
	dc, err := testdataclient.NewDoc(`Path("/some/path") -> "https://www.example.org"`)
	if err != nil || dc == nil {
		log.Fatal(err, dc == nil)
	}
}

func DisabledExampleClient_Update() {
	// create a data client:
	dc := testdataclient.New([]*eskip.Route{
		{Id: "route1", Path: "/some/path", Backend: "https://www1.example.org"},
		{Id: "route2", Path: "/some/path", Backend: "https://www2.example.org"}})

	// check initial routes:
	routes, err := dc.LoadAll()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("before update:")
	for _, r := range routes {
		fmt.Println(r.Backend)
	}

	// send an update:
	go func() {
		dc.Update(
			[]*eskip.Route{{Id: "route1", Path: "/some/path", Backend: "https://mod.example.org"}},
			[]string{"route2"})
	}()

	// receive the update:
	routes, deletedIds, err := dc.LoadUpdate()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("update:")
	for _, r := range routes {
		fmt.Println(r.Backend)
	}
	for _, id := range deletedIds {
		fmt.Println(id)
	}

	// check all routes again:
	routes, err = dc.LoadAll()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println()
	fmt.Println("after update:")
	for _, r := range routes {
		fmt.Println(r.Backend)
	}

	// Output:
	//
	// before update:
	// https://www1.example.org
	// https://www2.example.org
	//
	// update:
	// https://mod.example.org
	// route2
	//
	// after update:
	// https://mod.example.org
}

func ExampleClient_FailNext() {
	// create a data client:
	dc, err := testdataclient.NewDoc(`Path("/some/path") -> "https://www.example.org"`)
	if err != nil || dc == nil {
		log.Fatal(err, dc == nil)
	}

	// set the the next two requests to fail:
	dc.FailNext()
	dc.FailNext()

	// wait for the third request to succeed:
	_, err = dc.LoadAll()
	fmt.Println(err)

	_, err = dc.LoadAll()
	fmt.Println(err)

	routes, err := dc.LoadAll()
	if err != nil || len(routes) != 1 {
		log.Fatal(err, len(routes))
	}

	fmt.Println(routes[0].Backend)

	// Output:
	// failed to get routes
	// failed to get routes
	// https://www.example.org
}
