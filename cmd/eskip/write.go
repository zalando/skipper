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
	"github.com/zalando/skipper/eskip"
	etcdclient "github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters/flowid"
	"regexp"
)

const randomIdLength = 16

type (
	routeCond     func(*eskip.Route) bool
	twoRoutesCond func(left, right *eskip.Route) bool
	routeMap      map[string]*eskip.Route
)

var routeIdRx = regexp.MustCompile("\\W")

func any(_ *eskip.Route) bool { return true }

func routeDiffers(left, right *eskip.Route) bool {
	return left.String() != right.String()
}

func mapRoutes(routes routeList) routeMap {
	m := make(routeMap)
	for _, r := range routes {
		m[r.Id] = r
	}

	return m
}

// generate weak random id for a route if
// it doesn't have one.
func ensureId(r *eskip.Route) error {
	if r.Id != "" {
		return nil
	}

	id, err := flowid.NewFlowId(randomIdLength)
	if err != nil {
		return err
	}

	// replace characters that are not allowed
	// for eskip route ids.
	id = routeIdRx.ReplaceAllString(id, "x")
	r.Id = "route" + id
	return nil
}

// insert/update all routes to a medium (currently only etcd).
func upsertAll(routes routeList, m *medium) error {
	client := etcdclient.New(urlsToStrings(m.urls), m.path)
	for _, r := range routes {
		ensureId(r)
		err := client.Upsert(r)
		if err != nil {
			return err
		}
	}

	return nil
}

// take items from 'routes' that don't exist in 'ref' or fulfil 'altCond'.
func takeDiff(ref routeList, routes routeList, altCond twoRoutesCond) routeList {
	mref := mapRoutes(ref)
	var diff routeList
	for _, r := range routes {
		if rr, exists := mref[r.Id]; !exists || altCond(rr, r) {
			diff = append(diff, r)
		}
	}

	return diff
}

// insert/update routes from 'update' that don't exist in 'existing' or
// are different from the one with the same id in 'existing'.
func upsertDifferent(existing routeList, update routeList, m *medium) error {
	diff := takeDiff(existing, update, routeDiffers)
	return upsertAll(diff, m)
}

// delete all items in 'routes' that fulfil 'cond'.
func deleteAllIf(routes routeList, m *medium, cond routeCond) error {
	client := etcdclient.New(urlsToStrings(m.urls), m.path)
	for _, r := range routes {
		if !cond(r) {
			continue
		}

		err := client.Delete(r.Id)
		if err != nil {
			return err
		}
	}

	return nil
}

// command executed for upsert.
func upsertCmd(in, out *medium) error {
	// take input routes:
	routes, err := loadRoutesChecked(in)
	if err != nil {
		return err
	}

	// upsert routes that don't exist or are different:
	return upsertAll(routes, out)
}

// command executed for reset.
func resetCmd(in, out *medium) error {
	// take input routes:
	routes, err := loadRoutesChecked(in)
	if err != nil {
		return err
	}

	// take existing routes from output:
	existing := loadRoutesUnchecked(out)

	// upsert routes that don't exist or are different:
	err = upsertDifferent(existing, routes, out)
	if err != nil {
		return err
	}

	// delete routes from existing that were not upserted:
	rm := mapRoutes(routes)
	notSet := func(r *eskip.Route) bool {
		_, set := rm[r.Id]
		return !set
	}

	return deleteAllIf(existing, out, notSet)
}

// command executed for delete.
func deleteCmd(in, out *medium) error {
	// take input routes:
	routes, err := loadRoutesChecked(in)
	if err != nil {
		return err
	}

	// delete them:
	return deleteAllIf(routes, out, any)
}
