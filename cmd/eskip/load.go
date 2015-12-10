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

package main

import (
	"errors"
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/eskipfile"
	etcdclient "github.com/zalando/skipper/etcd"
	innkeeperclient "github.com/zalando/skipper/innkeeper"
	"io"
	"io/ioutil"
	"net/url"
	"os"
)

type loadResult struct {
	routes      routeList
	parseErrors map[string]error
}

var invalidRouteExpression = errors.New("one or more invalid route expressions")

// store all loaded routes, even if invalid, and store the
// parse errors if any.
func mapRouteInfo(allInfo []*etcdclient.RouteInfo) loadResult {
	lr := loadResult{make(routeList, len(allInfo)), make(map[string]error)}
	for i, info := range allInfo {
		lr.routes[i] = &info.Route
		if info.ParseError != nil {
			lr.parseErrors[info.Id] = info.ParseError
		}
	}

	return lr
}

// load and parse routes from a reader (used for stdin).
func loadReader(r io.Reader) (loadResult, error) {

	// this pretty much disables continuous piping,
	// but since the reset command first upserts all
	// and deletes the diff only after, it may not
	// even be consistent to do continous piping.
	// May change in the future.
	doc, err := ioutil.ReadAll(r)
	if err != nil {
		return loadResult{}, err
	}

	routes, err := eskip.Parse(string(doc))
	return loadResult{routes: routes}, err
}

// load and parse routes from a file using the eskipfile client.
func loadFile(path string) (loadResult, error) {
	client, err := eskipfile.Open(path)
	if err != nil {
		return loadResult{}, err
	}

	routes, err := client.LoadAll()
	return loadResult{routes: routes}, err
}

// load and parse routes from innkeeper
func loadInnkeeper(url *url.URL, oauthToken string) (loadResult, error) {
	auth := innkeeperclient.CreateInnkeeperAuthentication(innkeeperclient.AuthOptions{
		InnkeeperAuthToken: oauthToken})

	ic, err := innkeeperclient.New(innkeeperclient.Options{
		Address:        url.String(),
		Insecure:       true,
		Authentication: auth})

	if err != nil {
		return loadResult{}, err
	}

	routes, err := ic.LoadAll()

	return loadResult{routes: routes}, err
}

// load and parse routes from etcd.
func loadEtcd(urls []*url.URL, prefix string) (loadResult, error) {
	client := etcdclient.New(urlsToStrings(urls), prefix)
	info, err := client.LoadAndParseAll()
	return mapRouteInfo(info), err
}

// parse routes from a string.
func loadString(doc string) (loadResult, error) {
	routes, err := eskip.Parse(doc)
	return loadResult{routes: routes}, err
}

// generate empty route objects from ids.
func loadIds(ids []string) (loadResult, error) {
	routes := make(routeList, len(ids))
	for i, id := range ids {
		routes[i] = &eskip.Route{Id: id}
	}

	return loadResult{routes: routes}, nil
}

// load routes from input medium.
func loadRoutes(in *medium) (loadResult, error) {
	switch in.typ {
	case stdin:
		return loadReader(os.Stdin)
	case file:
		return loadFile(in.path)
	case innkeeper:
		return loadInnkeeper(in.urls[0], in.oauthToken)
	case etcd:
		return loadEtcd(in.urls, in.path)
	case inline:
		return loadString(in.eskip)
	case inlineIds:
		return loadIds(in.ids)
	default:
		return loadResult{}, invalidInputType
	}
}

// print parse errors and return a generic error
// if any.
func checkParseErrors(lr loadResult) error {
	if len(lr.parseErrors) == 0 {
		return nil
	}

	for id, perr := range lr.parseErrors {
		printStderr(id, perr)
	}

	return invalidRouteExpression
}

// load, parse routes and print parse errors if any.
func loadRoutesChecked(m *medium) (routeList, error) {
	lr, err := loadRoutes(m)
	if err != nil {
		return nil, err
	}

	return lr.routes, checkParseErrors(lr)
}

// load and parse routes, ignore parse errors.
func loadRoutesUnchecked(m *medium) routeList {
	lr, _ := loadRoutes(m)
	return lr.routes
}

// command executed for check.
func checkCmd(in, _ *medium, _ *WriteClient) error {
	_, err := loadRoutesChecked(in)
	return err
}

// command executed for print.
func printCmd(in, _ *medium, _ *WriteClient) error {
	lr, err := loadRoutes(in)
	if err != nil {
		return err
	}

	for _, r := range lr.routes {
		if perr, hasError := lr.parseErrors[r.Id]; hasError {
			printStderr(r.Id, perr)
		} else {
			if r.Id == "" {
				fmt.Println(r.String())
			} else {
				fmt.Printf("%s: %s;\n", r.Id, r.String())
			}
		}
	}

	if len(lr.parseErrors) > 0 {
		return invalidRouteExpression
	}

	return nil
}
