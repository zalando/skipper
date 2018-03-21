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
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/etcd/etcdtest"
)

func TestMain(m *testing.M) {
	for _, arg := range os.Args {
		log.Printf("arg: %s", arg)
		if arg == "-test.short=true" {
			return
		}
	}

	err := etcdtest.Start()
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		err := etcdtest.Stop()
		if err != nil {
			log.Fatal(err)
		}
	}()

	os.Exit(m.Run())
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

func TestEndpointErrorsString(t *testing.T) {
	ee := &endpointErrors{errors: []error{errors.New("foo error")}}

	expected := "request to one or more endpoints failed;foo error"

	if ee.Error() != expected {
		t.Error("unexpected error message")
		return
	}
}

func TestReceivesError(t *testing.T) {
	c, err := New(Options{Endpoints: []string{"invalid url"}, Prefix: "/skippertest-invalid"})
	if err != nil {
		t.Error(err)
		return
	}

	_, err = c.LoadAll()
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestAllEndpointsFailed(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))

	s.Close()

	c, err := New(Options{Endpoints: []string{s.URL, s.URL}, Prefix: "/skippertest-invalid"})

	if err != nil {
		t.Error(err)
		return
	}

	_, err = c.LoadAll()

	if err == nil {
		t.Error("failed to fail")
	}

	_, ok := err.(*endpointErrors)

	if !ok {
		t.Error("when all endpoints fail - endpointErrors must be returned")
	}
}

func TestFailedEndpointsRotation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))

	s.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer s2.Close()

	c, err := New(Options{Endpoints: []string{s.URL, s2.URL, "neverreached"}, Prefix: "/skippertest-invalid"})

	if err != nil {
		t.Error(err)
		return
	}

	_, err = c.LoadAll()

	if err == nil {
		t.Error("failed to fail")
	}

	expectedEndpoints := []string{s2.URL, "neverreached", s.URL}

	if strings.Join(c.endpoints, ";") != strings.Join(expectedEndpoints, ";") {
		t.Error("wrong endpoints rotation")
	}
}

func TestTimedoutEndpointsRotation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))

	s.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(5 * time.Millisecond))
	}))
	defer s2.Close()

	c, err := New(Options{Endpoints: []string{s.URL, s2.URL, "neverreached"}, Prefix: "/skippertest-invalid"})
	c.client.Timeout = time.Duration(1 * time.Millisecond)

	if err != nil {
		t.Error(err)
		return
	}

	_, err = c.LoadAll()

	if err == nil {
		t.Error("failed to fail")
	}

	nerr, ok := err.(net.Error)

	if !ok || !nerr.Timeout() {
		t.Error("timeout error expected")
	}

	expectedEndpoints := []string{s2.URL, "neverreached", s.URL}

	if strings.Join(c.endpoints, ";") != strings.Join(expectedEndpoints, ";") {
		t.Error("wrong endpoints rotation")
	}
}

func TestValidatesDocument(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"value": "different json"}`))
	}))
	defer s.Close()

	c, err := New(Options{Endpoints: []string{s.URL}, Prefix: "/skippertest-invalid"})
	if err != nil {
		t.Error(err)
		return
	}

	_, err = c.LoadAll()
	if err != invalidResponseDocument {
		t.Error("failed to fail")
	}
}

func TestReceivesInitial(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	if err := etcdtest.ResetData(); err != nil {
		t.Error(t)
		return
	}

	expectedEndpoints := strings.Join(etcdtest.Urls, ";")

	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	rs, err := c.LoadAll()

	if err != nil {
		t.Error(err)
	}

	if !checkInitial(rs) {
		t.Error("failed to receive the right docs")
	}

	if strings.Join(c.endpoints, ";") != expectedEndpoints {
		t.Error("wrong endpoints rotation")
	}
}

func TestReceivesUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	if err := etcdtest.ResetData(); err != nil {
		t.Error(err)
		return
	}

	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	c.LoadAll()

	etcdtest.PutData("pdp", `Path("/pdp") -> "https://updated.example.org"`)

	rs, ds, err := c.LoadUpdate()
	if err != nil {
		t.Error(err)
	}

	if !checkBackend(rs, "pdp", "https://updated.example.org") {
		t.Error("failed to receive the right backend", len(rs))
	}

	if len(ds) != 0 {
		t.Error("unexpected delete")
	}
}

func TestReceiveInsert(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	if err := etcdtest.ResetData(); err != nil {
		t.Error(err)
		return
	}

	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	_, err = c.LoadAll()
	if err != nil {
		t.Error(err)
	}

	etcdtest.PutData("catalog", `Path("/pdp") -> "https://catalog.example.org"`)

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
	if testing.Short() {
		t.Skip()
	}
	if err := etcdtest.ResetData(); err != nil {
		t.Error(err)
		return
	}

	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	c.LoadAll()

	etcdtest.DeleteData("pdp")

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
	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.Upsert(&eskip.Route{})
	if err != missingRouteId {
		t.Error("failed to fail")
	}
}

func TestUpsertNew(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	if err := etcdtest.DeleteAll(); err != nil {
		t.Error(err)
		return
	}

	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.Upsert(&eskip.Route{
		Id:     "route1",
		Method: "POST",
		Shunt:  true})
	if err != nil {
		t.Error(err)
	}

	routes, err := c.LoadAll()
	if len(routes) != 1 || routes[0].Id != "route1" {
		t.Error("failed to upsert route", len(routes))
	}
}

func TestUpsertExisting(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	if err := etcdtest.DeleteAll(); err != nil {
		t.Error(err)
		return
	}

	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.Upsert(&eskip.Route{
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
	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.Delete("")
	if err != missingRouteId {
		t.Error("failed to fail")
	}
}

func TestDeleteNotExists(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	if err := etcdtest.DeleteAll(); err != nil {
		t.Error(err)
		return
	}
	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.Upsert(&eskip.Route{
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
	if testing.Short() {
		t.Skip()
	}

	if err := etcdtest.DeleteAll(); err != nil {
		t.Error(err)
		return
	}
	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.Upsert(&eskip.Route{
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
	if testing.Short() {
		t.Skip()
	}

	if err := etcdtest.DeleteAll(); err != nil {
		t.Error(err)
		return
	}

	etcdtest.PutData("catalog", `Path("/pdp") -> "https://catalog.example.org"`)
	etcdtest.PutData("cms", "invalid expression")

	c, err := New(Options{etcdtest.Urls, "/skippertest", 0, false, ""})
	if err != nil {
		t.Error(err)
		return
	}

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
