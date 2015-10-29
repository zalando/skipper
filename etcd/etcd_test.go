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

package etcd

import (
	"github.com/coreos/go-etcd/etcd"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/etcd/etcdtest"
	"log"
	"testing"
)

func init() {
	// start an etcd server
	err := etcdtest.Start()
	if err != nil {
		log.Fatal(err)
	}
}

func checkInitial(d []*eskip.Route) bool {
	if len(d) != 1 {
		return false
	}

	r := d[0]

	if r.Id != "pdp" {
		return false
	}

	if len(r.PathRegexps) != 1 || r.PathRegexps[0] != ".*\\.html" {
		return false
	}

	if len(r.Filters) != 2 {
		return false
	}

	checkFilter := func(f *eskip.Filter, name string, args ...interface{}) bool {
		if f.Name != name {
			return false
		}

		if len(f.Args) != len(args) {
			return false
		}

		for i, a := range args {
			if f.Args[i] != a {
				return false
			}
		}

		return true
	}

	if !checkFilter(r.Filters[0], "customHeader", 3.14) {
		return false
	}

	if !checkFilter(r.Filters[1], "xSessionId", "s4") {
		return false
	}

	if r.Backend != "https://www.example.org" {
		return false
	}

	return true
}

func checkBackend(d []*eskip.Route, routeId, backend string) bool {
	for _, r := range d {
		if r.Id == routeId {
			return r.Backend == backend
		}
	}

	return false
}

func checkDeleted(ids []string, routeId string) bool {
	for _, id := range ids {
		if id == routeId {
			return true
		}
	}

	return false
}

func deleteData() {
	c := etcd.NewClient(etcdtest.Urls)

	// for the tests, considering errors on delete as not-found
	c.Delete("/skippertest", true)
}

func resetData(t *testing.T) {
	const testRoute = `
		PathRegexp(".*\\.html") ->
		customHeader(3.14) ->
		xSessionId("s4") ->
		"https://www.example.org"
	`

	deleteData()

	c := etcd.NewClient(etcdtest.Urls)
	_, err := c.Set("/skippertest/routes/pdp", testRoute, 0)
	if err != nil {
		t.Error(err)
		return
	}
}

func TestReceivesError(t *testing.T) {
	c := New(etcdtest.Urls, "/skippertest-invalid")
	_, err := c.LoadAll()
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestReceivesInitial(t *testing.T) {
	resetData(t)

	c := New(etcdtest.Urls, "/skippertest")
	rs, err := c.LoadAll()

	if err != nil {
		t.Error(err)
	}

	if !checkInitial(rs) {
		t.Error("failed to receive the right docs")
	}
}

func TestReceivesUpdates(t *testing.T) {
	resetData(t)

	c := New(etcdtest.Urls, "/skippertest")
	c.LoadAll()

	e := etcd.NewClient(etcdtest.Urls)
	_, err := e.Set("/skippertest/routes/pdp", `Path("/pdp") -> "https://updated.example.org"`, 0)
	if err != nil {
		t.Error(err)
	}

	rs, ds, err := c.LoadUpdate()
	if err != nil {
		t.Error(err)
	}

	if !checkBackend(rs, "pdp", "https://updated.example.org") {
		t.Error("failed to receive the right backend")
	}

	if len(ds) != 0 {
		t.Error("unexpected delete")
	}
}

func TestReceiveInsert(t *testing.T) {
	resetData(t)

	c := New(etcdtest.Urls, "/skippertest")
	_, err := c.LoadAll()
	if err != nil {
		t.Error(err)
	}

	e := etcd.NewClient(etcdtest.Urls)
	_, err = e.Set("/skippertest/routes/catalog", `Path("/pdp") -> "https://catalog.example.org"`, 0)
	if err != nil {
		t.Error(err)
	}

	rs, ds, err := c.LoadUpdate()
	if err != nil {
		t.Error(err)
	}

	if !checkBackend(rs, "catalog", "https://catalog.example.org") {
		t.Error("failed to receive the right backend")
	}

	if len(ds) != 0 {
		t.Error("unexpected delete")
	}
}

func TestReceiveDelete(t *testing.T) {
	resetData(t)

	c := New(etcdtest.Urls, "/skippertest")
	c.LoadAll()

	e := etcd.NewClient(etcdtest.Urls)
	e.Delete("/skippertest/routes/pdp", false)

	rs, ds, err := c.LoadUpdate()
	if err != nil {
		t.Error(err)
	}

	if !checkDeleted(ds, "pdp") {
		t.Error("failed to receive the right deleted id")
	}

	if len(rs) != 0 {
		t.Error("unexpected upsert")
	}
}

func TestUpsertNoId(t *testing.T) {
	c := New(etcdtest.Urls, "/skippertest")
	err := c.Upsert(&eskip.Route{})
	if err != missingRouteId {
		t.Error("failed to fail")
	}
}

func TestUpsertNew(t *testing.T) {
	deleteData()
	c := New(etcdtest.Urls, "/skippertest")

	err := c.Upsert(&eskip.Route{
		Id:     "route1",
		Method: "POST",
		Shunt:  true})
	if err != nil {
		t.Error(err)
	}

	routes, err := c.LoadAll()
	if len(routes) != 1 || routes[0].Id != "route1" {
		t.Error("failed to upsert route")
	}
}

func TestUpsertExisting(t *testing.T) {
	deleteData()
	c := New(etcdtest.Urls, "/skippertest")

	err := c.Upsert(&eskip.Route{
		Id:     "route1",
		Method: "POST",
		Shunt:  true})
	if err != nil {
		t.Error(err)
	}

	err = c.Upsert(&eskip.Route{
		Id:     "route1",
		Method: "PUT",
		Shunt:  true})
	if err != nil {
		t.Error(err)
	}

	routes, err := c.LoadAll()
	if len(routes) != 1 || routes[0].Method != "PUT" {
		t.Error("failed to upsert route")
	}
}

func TestDeleteNoId(t *testing.T) {
	c := New(etcdtest.Urls, "/skippertest")
	err := c.Delete("")
	if err != missingRouteId {
		t.Error("failed to fail")
	}
}

func TestDeleteNotExists(t *testing.T) {
	deleteData()
	c := New(etcdtest.Urls, "/skippertest")

	err := c.Upsert(&eskip.Route{
		Id:     "route1",
		Method: "POST",
		Shunt:  true})
	if err != nil {
		t.Error(err)
	}

	err = c.Delete("route1")
	if err != nil {
		t.Error(err)
	}

	err = c.Delete("route1")
	if err != nil {
		t.Error(err)
	}
}

func TestDelete(t *testing.T) {
	deleteData()
	c := New(etcdtest.Urls, "/skippertest")

	err := c.Upsert(&eskip.Route{
		Id:     "route1",
		Method: "POST",
		Shunt:  true})
	if err != nil {
		t.Error(err)
	}

	err = c.Delete("route1")
	if err != nil {
		t.Error(err)
	}

	routes, err := c.LoadAll()
	if len(routes) != 0 {
		t.Error("failed to delete route")
	}
}

func TestLoadWithParseFailures(t *testing.T) {
	deleteData()
	e := etcd.NewClient(etcdtest.Urls)

	_, err := e.Set("/skippertest/routes/catalog", `Path("/pdp") -> "https://catalog.example.org"`, 0)
	if err != nil {
		t.Error(err)
	}

	_, err = e.Set("/skippertest/routes/cms", "invalid expression", 0)
	if err != nil {
		t.Error(err)
	}

	c := New(etcdtest.Urls, "/skippertest")
	routeInfo, err := c.LoadAndParseAll()
	if err != nil {
		t.Error(err)
	}

	if len(routeInfo) != 2 {
		t.Error("failed to load all routes", len(routeInfo))
	}

	var parseError error
	for _, ri := range routeInfo {
		if ri.ParseError != nil {
			if parseError != nil {
				t.Error("too many errors")
			}

			parseError = ri.ParseError
		}
	}

	if parseError == nil {
		t.Error("failed to detect parse error")
	}
}
