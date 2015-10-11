// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy_test

import (
	"fmt"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
)

// custom filter type:
type setTestResponse struct{}

func (s *setTestResponse) Name() string                                         { return "setTestResponse" }
func (s *setTestResponse) CreateFilter(_ []interface{}) (filters.Filter, error) { return s, nil }
func (f *setTestResponse) Response(_ filters.FilterContext)                     {}

// the filter copies the path parameter 'response' to the 'X-Response' header
func (f *setTestResponse) Request(ctx filters.FilterContext) {
	ctx.Request().Header.Set("X-Response", ctx.PathParam("response"))
}

func Example() {
	// create a target server. It will return the value of the 'X-Response' header as the response body:
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Header.Get("X-Response")))
	}))

	defer targetServer.Close()

	// create a filter registry, and register the custom filter:
	filterRegistry := filters.Defaults()
	filterRegistry.Register(&setTestResponse{})

	// create a data client with a predefined route, referencing the filter:
	routeDoc := fmt.Sprintf(`Path("/return/:response") -> setTestResponse() -> "%s"`, targetServer.URL)
	dataClient, err := testdataclient.NewDoc(routeDoc)
	if err != nil {
		log.Fatal(err)
	}

	// create a proxy instance, and start an http server:
	proxy := proxy.New(routing.New(routing.Options{
		FilterRegistry: filterRegistry,
		DataClients:    []routing.DataClient{dataClient}}), false)
	routerServer := httptest.NewServer(proxy)
	defer routerServer.Close()

	// make a request to the proxy:
	rsp, err := http.Get(fmt.Sprintf("%s/return/Hello,+world!", routerServer.URL))
	if err != nil {
		log.Fatal(err)
	}

	defer rsp.Body.Close()

	// print out the response:
	data, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(data))

	// Output:
	// Hello, world!
}
