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

/*
Package proxy implements an HTTP reverse proxy based on continuously
updated skipper routing rules.

The proxy matches each incoming request to the lookup tree for the first
matching route, and handles it accordingly to the rules defined in it.
This typically means augmenting the request with the filters and
forwarding it to the route endpoint, but it may also mean to handle the
request internally if it is a 'shunt' route.


Proxy Mechanism

1. route matching:

The incoming request is matched to the current routing tree, implemented
in skipper/routing. The result may be a route, which will be used for
forwarding or handling the request, or nil, in which case the proxy
responds with 404.


2. upstream request augmentation:

In case of a matched route, the request handling method of all filters
in the route will be executed in the order they are defined. The filters
share a context object, that provides the in-memory representation of the
incoming request, the outgoing response writer, the path parameters
derived from the actual request path (see skipper/routing) and a
free-form state bag. The filters may modify the request or pass data to
each other using the state bag.


3.a upstream request:

The incoming and augmented request is mapped to an outgoing request and
executed, addressing the endpoint defined by the current route.


3.b shunt:

In case the route is a 'shunt', an empty response is created with
default 404 status.


4. downstream response augmentation:

The response handling method of all filters in the current route
definition will be executed, but this time in reverse order. The filter
context is the same instance as the one in step 2, but this time it
includes the response object from step 3. If the route is a shunt route,
one of the filters needs to handle the request latest in this phase by
setting the right status and response headers, and writing the response
body, if any, to the writer in the filter context, and mark the request
as 'served'.


5. response:

In case none of the filters handled the request, the response
properties, including the status and the headers, are mapped to the
outgoing response writer, and the response body is streamed to it, with
continuous flushing.


Routing Rules

The route matching is implemented in the skipper/routing package. The
routing rules are not static, but they can be continuously updated by
new definitions originated in one or more data sources.

The only exceptions are the priority routes, that have not originated
from the external data sources, and are tested against the requests
before the general routing tree.


Example

The below example demonstrates creating a routing proxy as a standard
http.Handler interface:

	// create a target backend server. It will return the value of the 'X-Echo' request header
	// as the response body:
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Header.Get("X-Echo")))
	}))

	defer targetServer.Close()

	// create a filter registry, and register the custom filter:
	filterRegistry := builtin.MakeRegistry()
	filterRegistry.Register(&setEchoHeader{})

	// create a data client with a predefined route, referencing the filter and a path condition
	// containing a wildcard called 'echo':
	routeDoc := fmt.Sprintf(`Path("/return/:echo") -> setEchoHeader() -> "%s"`, targetServer.URL)
	dataClient, err := testdataclient.NewDoc(routeDoc)
	if err != nil {
		log.Fatal(err)
	}

	// create a proxy instance, and start an http server:
	proxy := proxy.New(routing.New(routing.Options{
		FilterRegistry: filterRegistry,
		DataClients:    []routing.DataClient{dataClient}}), proxy.OptionsNone)
	router := httptest.NewServer(proxy)
	defer router.Close()

	// make a request to the proxy:
	rsp, err := http.Get(fmt.Sprintf("%s/return/Hello,+world!", router.URL))
	if err != nil {
		log.Fatal(err)
	}

	defer rsp.Body.Close()

	// print out the response:
	if _, err := io.Copy(os.Stdout, rsp.Body); err != nil {
		log.Fatal(err)
	}

	// Output:
	// Hello, world!
*/
package proxy
