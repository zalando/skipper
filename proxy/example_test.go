package proxy_test

import (
	"log"
	"net/http"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

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
	routeDoc := `* -> "https://www.example.org"`
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
		pr)
}
