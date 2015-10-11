package etcd_test

import (
	"github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

func Example() {
	// create etcd data client:
	dataClient := etcd.New([]string{"https://etcd.example.org"}, "/skipper")

	// create http.Handler:
	proxy.New(
		routing.New(routing.Options{
			FilterRegistry: filters.Defaults(),
			DataClients:    []routing.DataClient{dataClient}}),
		false)
}
