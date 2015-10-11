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
