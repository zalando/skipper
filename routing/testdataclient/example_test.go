package testdataclient_test

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func Example() {
	// create a data client:
	dataClient := testdataclient.New([]*eskip.Route{
		{Path: "/some/path", Backend: "https://www.example.org"}})

	// (only in tests)
	tl := loggingtest.New()
	defer tl.Close()

	// create a router:
	r := routing.New(routing.Options{
		DataClients: []routing.DataClient{dataClient},
		Log:         tl})
	defer r.Close()

	// wait for the route data being propagated:
	tl.WaitFor("route settings applied", time.Second)

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

func ExampleClient_FailNext() {
	// create a data client:
	dc, err := testdataclient.NewDoc(`Path("/some/path") -> "https://www.example.org"`)
	if err != nil || dc == nil {
		log.Fatal(err, dc == nil)
	}

	// set the next two requests to fail:
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
