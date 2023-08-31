package routestring_test

import (
	"fmt"
	"io"
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
	time.Sleep(10 * time.Millisecond)

	rsp, err := http.Get("http://localhost:9999")
	if err != nil {
		log.Println(err)
		return
	}

	defer rsp.Body.Close()
	content, err := io.ReadAll(rsp.Body)
	if err != nil {
		log.Println(err)
		return
	}

	fmt.Println(string(content))

	// Output:
	// Hello, world!
}
