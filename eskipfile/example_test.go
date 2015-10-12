package eskipfile_test

import (
	"github.com/zalando/skipper/eskipfile"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"log"
)

func Example() {
	// open file with routing table:
	dataClient, err := eskipfile.Open("/some/path/to/routing-table.eskip")
	if err != nil {
		log.Fatal(err)
	}

	// create http.Handler:
	proxy.New(
		routing.New(routing.Options{
			DataClients: []routing.DataClient{dataClient}}),
		false)
}
