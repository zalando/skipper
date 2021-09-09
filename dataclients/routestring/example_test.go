//go:build !race
// +build !race

package routestring_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/zalando/skipper"
	"github.com/zalando/skipper/dataclients/routestring"
	"github.com/zalando/skipper/routing"
)

func Example() {
	rs, err := routestring.New(`* -> inlineContent("Hello, world!") -> <shunt>`)
	if err != nil {
		log.Println(err)
		return
	}

	go func() {
		skipper.Run(skipper.Options{
			Address:           ":9999",
			CustomDataClients: []routing.DataClient{rs},
		})
	}()
	time.Sleep(1 * time.Millisecond)

	rsp, err := http.Get("http://localhost:9999")
	if err != nil {
		log.Println(err)
		return
	}

	defer rsp.Body.Close()
	content, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		log.Println(err)
		return
	}

	fmt.Println(string(content))

	// Output:
	// Hello, world!
}
