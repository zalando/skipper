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
	"log"
	"net/url"
	"os"
	"slices"
	"testing"

	"github.com/zalando/skipper/etcd/etcdtest"
)

var testEtcdUrls []*url.URL

func TestMain(m *testing.M) {
	if slices.Contains(os.Args, "-test.short=true") {
		return
	}

	err := etcdtest.StartProjectRoot("../..")
	if err != nil {
		log.Fatal(err)
	}

	urls, err := stringsToUrls(etcdtest.Urls...)
	if err != nil {
		log.Fatal(err)
	}

	testEtcdUrls = urls

	exitCode := m.Run()

	err = etcdtest.Stop()
	if err != nil {
		log.Fatal(err)
	}

	os.Exit(exitCode)
}

func TestUpsertLoadFail(t *testing.T) {
	in := &medium{typ: inline, eskip: "invalid doc"}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdPrefix}

	err := upsertCmd(cmdArgs{in: in, out: out})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestUpsertGeneratesId(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	etcdtest.DeleteAllFrom(defaultEtcdPrefix)

	in := &medium{typ: inline, eskip: `Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdPrefix}
	err := upsertCmd(cmdArgs{in: in, out: out})
	if err != nil {
		t.Error(err)
	}

	routes, err := loadRoutesChecked(out)
	if err != nil {
		t.Error(err)
	}

	if len(routes) != 1 || routes[0].Id == "" {
		t.Error("upsert failed")
	}
}

func TestUpsertUsesId(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	etcdtest.DeleteAllFrom(defaultEtcdPrefix)

	in := &medium{typ: inline, eskip: `route1: Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdPrefix}
	err := upsertCmd(cmdArgs{in: in, out: out})
	if err != nil {
		t.Error(err)
	}

	routes, err := loadRoutesChecked(out)
	if err != nil {
		t.Error(err)
	}

	if len(routes) != 1 || routes[0].Id != "route1" {
		t.Error("upsert failed")
	}
}

func TestResetLoadFail(t *testing.T) {
	in := &medium{typ: inline, eskip: "invalid doc"}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdPrefix}
	err := resetCmd(cmdArgs{in: in, out: out})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestResetLoadExistingFails(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	etcdtest.DeleteAllFrom(defaultEtcdPrefix)

	in := &medium{typ: inline, eskip: `route2: Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdPrefix}
	err := etcdtest.PutDataTo(defaultEtcdPrefix, "route1", "invalid doc")
	if err != nil {
		t.Error(err)
	}

	err = resetCmd(cmdArgs{in: in, out: out})
	if err != nil {
		t.Error(err)
	}

	_, err = etcdtest.GetNodeFrom(defaultEtcdPrefix, "route1")
	if err == nil {
		t.Error(err)
	}

	_, err = etcdtest.GetNodeFrom(defaultEtcdPrefix, "route2")
	if err != nil {
		t.Error(err)
	}
}

func TestReset(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	etcdtest.DeleteAllFrom(defaultEtcdPrefix)

	in := &medium{typ: inline, eskip: `route2: Method("PUT") -> <shunt>; route3: Method("HEAD") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdPrefix}
	err := etcdtest.PutDataTo(defaultEtcdPrefix, "route1", `Method("GET") -> <shunt>`)
	if err != nil {
		t.Error(err)
	}

	err = etcdtest.PutDataTo(defaultEtcdPrefix, "route2", `Method("POST") -> <shunt>`)
	if err != nil {
		t.Error(err)
	}

	err = resetCmd(cmdArgs{in: in, out: out})
	if err != nil {
		t.Error(err)
	}

	routes, err := loadRoutesChecked(out)
	if err != nil {
		t.Error(err)
	}

	if len(routes) != 2 {
		t.Error("failed to reset routes")
	}

	for _, id := range []string{"route2", "route3"} {
		found := false
		for _, r := range routes {
			if r.Id != id {
				continue
			}

			found = true
			switch id {
			case "route2":
				if r.Method != "PUT" {
					t.Error("failed to reset routes")
				}
			case "route3":
				if r.Method != "HEAD" {
					t.Error("failed to reset routes")
				}
			}
		}

		if !found {
			t.Error("failed to reset routes")
		}
	}
}

func TestDeleteLoadFails(t *testing.T) {
	in := &medium{typ: inline, eskip: "invalid doc"}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdPrefix}
	err := deleteCmd(cmdArgs{in: in, out: out})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestDeleteFromIds(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	etcdtest.DeleteAllFrom(defaultEtcdPrefix)

	in := &medium{typ: inline, eskip: `route1: Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdPrefix}
	err := upsertCmd(cmdArgs{in: in, out: out})
	if err != nil {
		t.Error(err)
	}

	in = &medium{typ: inlineIds, ids: []string{"route1", "route2"}}
	err = deleteCmd(cmdArgs{in: in, out: out})
	if err != nil {
		t.Error(err)
	}

	routes, err := loadRoutesChecked(out)
	if err != nil {
		t.Error(err)
	}

	if len(routes) != 0 {
		t.Error("delete failed")
	}
}

func TestDeleteFromRoutes(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	etcdtest.DeleteAllFrom(defaultEtcdPrefix)

	in := &medium{typ: inline, eskip: `route1: Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdPrefix}
	err := upsertCmd(cmdArgs{in: in, out: out})
	if err != nil {
		t.Error(err)
	}

	in = &medium{typ: inline, eskip: `route1: Method("HEAD") -> <shunt>;route2: Method("PUT") -> <shunt>`}
	err = deleteCmd(cmdArgs{in: in, out: out})
	if err != nil {
		t.Error(err)
	}

	routes, err := loadRoutesChecked(out)
	if err != nil {
		t.Error(err)
	}

	if len(routes) != 0 {
		t.Error("delete failed")
	}
}
