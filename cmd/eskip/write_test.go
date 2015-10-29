package main

import (
	etcdclient "github.com/coreos/go-etcd/etcd"
	"github.com/zalando/skipper/etcd/etcdtest"
	"log"
	"net/url"
	"testing"
)

var testEtcdUrls []*url.URL

func init() {
	etcdtest.Start()
	urls, err := parseUrls(etcdtest.Urls)
	if err != nil {
		log.Fatal(err)
	}

	testEtcdUrls = urls
}

func TestUpsertLoadFail(t *testing.T) {
	in := &medium{typ: inline, eskip: "invalid doc"}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdStorageRoot}
	err := upsertCmd(in, out)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestUpsertGeneratesId(t *testing.T) {
	deleteRoutesFrom(defaultEtcdStorageRoot)

	in := &medium{typ: inline, eskip: `Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdStorageRoot}

	err := upsertCmd(in, out)
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
	deleteRoutesFrom(defaultEtcdStorageRoot)

	in := &medium{typ: inline, eskip: `route1: Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdStorageRoot}

	err := upsertCmd(in, out)
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
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdStorageRoot}
	err := resetCmd(in, out)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestResetLoadExistingFails(t *testing.T) {
	deleteRoutesFrom(defaultEtcdStorageRoot)

	in := &medium{typ: inline, eskip: `route2: Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdStorageRoot}

	c := etcdclient.NewClient(etcdtest.Urls)
	_, err := c.Set(defaultEtcdStorageRoot+"/routes/route1", "invalid doc", 0)
	if err != nil {
		t.Error(err)
	}

	err = resetCmd(in, out)
	if err != nil {
		t.Error(err)
	}

	_, err = c.Get(defaultEtcdStorageRoot+"/routes/route1", false, false)
	if err == nil {
		t.Error(err)
	}

	_, err = c.Get(defaultEtcdStorageRoot+"/routes/route2", false, false)
	if err != nil {
		t.Error(err)
	}
}

func TestReset(t *testing.T) {
	deleteRoutesFrom(defaultEtcdStorageRoot)

	in := &medium{typ: inline, eskip: `route2: Method("PUT") -> <shunt>; route3: Method("HEAD") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdStorageRoot}

	c := etcdclient.NewClient(etcdtest.Urls)

	_, err := c.Set(defaultEtcdStorageRoot+"/routes/route1", `Method("GET") -> <shunt>`, 0)
	if err != nil {
		t.Error(err)
	}

	_, err = c.Set(defaultEtcdStorageRoot+"/routes/route2", `Method("POST") -> <shunt>`, 0)
	if err != nil {
		t.Error(err)
	}

	err = resetCmd(in, out)
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
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdStorageRoot}
	err := deleteCmd(in, out)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestDeleteFromIds(t *testing.T) {
	deleteRoutesFrom(defaultEtcdStorageRoot)

	in := &medium{typ: inline, eskip: `route1: Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdStorageRoot}

	err := upsertCmd(in, out)
	if err != nil {
		t.Error(err)
	}

	in = &medium{typ: inlineIds, ids: []string{"route1", "route2"}}
	err = deleteCmd(in, out)
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
	deleteRoutesFrom(defaultEtcdStorageRoot)

	in := &medium{typ: inline, eskip: `route1: Method("POST") -> <shunt>`}
	out := &medium{typ: etcd, urls: testEtcdUrls, path: defaultEtcdStorageRoot}

	err := upsertCmd(in, out)
	if err != nil {
		t.Error(err)
	}

	in = &medium{typ: inline, eskip: `route1: Method("HEAD") -> <shunt>;route2: Method("PUT") -> <shunt>`}
	err = deleteCmd(in, out)
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
