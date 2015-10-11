package filtertest_test

import (
	"fmt"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"log"
)

type customFilter struct{}

func (f *customFilter) Request(ctx filters.FilterContext) {
	ctx.StateBag()["filter called"] = true
}

func (f *customFilter) Response(ctx filters.FilterContext) {}

func ExampleFilter() {
	// create a test filter and add to registry:
	fr := filters.Defaults()
	fr.Register(&filtertest.Filter{FilterName: "testFilter"})

	// create a data client, with a predefined route referencing the filter:
	dc, err := testdataclient.NewDoc(`Path("/some/path/:param") -> testFilter(3.14, "Hello, world!") -> "https://www.example.org"`)
	if err != nil {
		log.Fatal(err)
	}

	// create an http.Handler:
	proxy.New(
		routing.New(routing.Options{
			DataClients:    []routing.DataClient{dc},
			FilterRegistry: fr}),
		false)
}

func ExampleContext() {
	// create a filter instance:
	filter := &customFilter{}

	// create a test context:
	ctx := &filtertest.Context{FStateBag: make(map[string]interface{})}

	// call the request handler method of the filter:
	filter.Request(ctx)
	fmt.Printf("%t", ctx.StateBag()["filter called"].(bool))

	// Output:
	// true
}
