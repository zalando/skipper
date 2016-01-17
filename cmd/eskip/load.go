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
)

type loadResult struct {
	routes      []*eskip.Route
	parseErrors map[string]error
}

var invalidRouteExpression = errors.New("one or more invalid route expressions")

// store all loaded routes, even if invalid, and store the
// parse errors if any.
func mapRouteInfo(allInfo []*eskip.RouteInfo) loadResult {
	lr := loadResult{make([]*eskip.Route, len(allInfo)), make(map[string]error)}
	for i, info := range allInfo {
		lr.routes[i] = &info.Route
		if info.ParseError != nil {
			lr.parseErrors[info.Id] = info.ParseError
		}
	}

	return lr
}

// load routes from input medium.
func loadRoutes(readClient readClient) (loadResult, error) {

	routeInfos, err := readClient.LoadAndParseAll()

	return mapRouteInfo(routeInfos), err
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
func loadRoutesChecked(readClient readClient) ([]*eskip.Route, error) {
	lr, err := loadRoutes(readClient)
	if err != nil {
		return nil, err
	}

	return lr.routes, checkParseErrors(lr)
}

// load and parse routes, ignore parse errors.
func loadRoutesUnchecked(readClient readClient) []*eskip.Route {
	lr, _ := loadRoutes(readClient)
	return lr.routes
}

// command executed for check.
func checkCmd(readClient readClient, _ readClient, _ writeClient) error {
	_, err := loadRoutesChecked(readClient)
	return err
}

// command executed for print.
func printCmd(readClient readClient, _ readClient, _ writeClient) error {
	lr, err := loadRoutes(readClient)
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
