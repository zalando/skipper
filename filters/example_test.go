package filters_test

import (
	"errors"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"log"
)

type customSpec struct{ name string }
type customFilter struct{ prefix string }

func (s *customSpec) Name() string {
	return s.name
}

// a specification can be used to create filter instances with different config
func (s *customSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) == 0 {
		return nil, errors.New("missing prefix argument for filter: customFilter")
	}

	prefix, ok := config[0].(string)
	if !ok {
		return nil, errors.New("invalid type of prefix argument for filter: customFilter")
	}

	return &customFilter{prefix}, nil
}

// a simple filter logging the request URLs
func (f *customFilter) Request(ctx filters.FilterContext) {
	log.Println(f.prefix, ctx.Request().URL)
}

func (f *customFilter) Response(_ filters.FilterContext) {}

func Example() {
	// create registry
	registry := filters.Defaults()

	// create and register the filter specification
	spec := &customSpec{name: "customFilter"}
	registry.Register(spec)

	// create simple data client, with route entries referencing 'customFilter',
	// and clipping part of the request path:
	dataClient, err := testdataclient.NewDoc(`

		ui: Path("/ui/*page") ->
			customFilter("ui request") ->
            modPath("^/[^/]*", "") ->
			"https://ui.example.org";

		api: Path("/api/*resource") ->
			customFilter("api request") ->
            modPath("^/[^/]*", "") ->
			"https://api.example.org"`)

	if err != nil {
		log.Fatal(err)
	}

	// create http.Handler, where all requests will be logged,
	// prefixed with the request type (ui or api):
	proxy.New(
		routing.New(routing.Options{
			FilterRegistry: registry,
			DataClients:    []routing.DataClient{dataClient}}),
		false)
}
