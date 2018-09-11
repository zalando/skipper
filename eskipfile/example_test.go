package eskipfile_test

import (
	"github.com/zalando/skipper/eskipfile"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

func Example() {
	// open file with a routing table:
	dataClient := eskipfile.Watch("/some/path/to/routing-table.eskip")
	defer dataClient.Close()

	// create a routing object:
	rt := routing.New(routing.Options{
		DataClients: []routing.DataClient{dataClient},
	})
	defer rt.Close()

	// create an http.Handler:
	p := proxy.New(rt, proxy.OptionsNone)
	defer p.Close()
}
