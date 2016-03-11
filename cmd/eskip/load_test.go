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
	"bytes"
	"errors"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/etcd/etcdtest"
	"log"
	"os"
	"strings"
	"testing"
)

const testStdinName = "testStdin"

type testClient string

func (tc testClient) LoadAndParseAll() ([]*eskip.RouteInfo, error) {
	routes, err := eskip.Parse(string(tc))
	if err != nil {
		return nil, err
	}

	routesInfo := make([]*eskip.RouteInfo, len(routes))
	for i, r := range routes {
		routesInfo[i] = &eskip.RouteInfo{Route: *r}
	}

	return routesInfo, err
}

var ioError = errors.New("io error")

func init() {
	// start an etcd server
	err := etcdtest.Start()
	if err != nil {
		log.Fatal(err)
	}
}

func preserveStdin(f *os.File, action func()) {
	f, os.Stdin = os.Stdin, f
	defer func() { os.Stdin = f }()
	action()
}

func withFile(name string, content string, action func(f *os.File)) error {
	var (
		err error
		f   *os.File
	)

	withError := func(action func()) {
		if err != nil {
			return
		}

		action()
	}

	func() {
		withError(func() { f, err = os.Create(name) })
		if err == nil {
			defer f.Close()
		}

		withError(func() { _, err = f.Write([]byte(content)) })
		withError(func() { _, err = f.Seek(0, 0) })
		action(f)
	}()

	withError(func() { err = os.Remove(name) })

	if err == nil {
		return nil
	}

	return ioError
}

func withStdin(content string, action func()) error {
	return withFile(testStdinName, content, func(f *os.File) {
		preserveStdin(f, action)
	})
}

func TestCheckStdinInvalid(t *testing.T) {

	err := withStdin("invalid doc", func() {
		readClient, _ := createReadClient(&medium{typ: stdin})

		err := checkCmd(readClient, nil, nil, nil)
		if err == nil {
			t.Error("failed to fail")
		}
	})

	if err != nil {
		t.Error(err)
	}
}

func TestCheckStdin(t *testing.T) {
	err := withStdin(`Method("POST") -> "https://www.example.org"`, func() {
		readClient, _ := createReadClient(&medium{typ: stdin})
		err := checkCmd(readClient, nil, nil, nil)
		if err != nil {
			t.Error(err)
		}
	})

	if err != nil {
		t.Error(err)
	}
}

func TestCheckFileInvalid(t *testing.T) {
	const name = "testFile"
	err := withFile(name, "invalid doc", func(_ *os.File) {
		_, err := createReadClient(&medium{typ: file, path: name})

		if err == nil {
			t.Error("failed to fail")
		}
	})

	if err != nil {
		t.Error(err)
	}
}

func TestCheckFile(t *testing.T) {
	const name = "testFile"
	err := withFile(name, `Method("POST") -> "https://www.example.org"`, func(_ *os.File) {
		readClient, _ := createReadClient(&medium{typ: file, path: name})
		err := checkCmd(readClient, nil, nil, nil)
		if err != nil {
			t.Error(err)
		}
	})

	if err != nil {
		t.Error(err)
	}
}

func TestCheckEtcdInvalid(t *testing.T) {
	urls, err := stringsToUrls(etcdtest.Urls...)
	if err != nil {
		t.Error(err)
	}

	etcdtest.DeleteAll()
	etcdtest.PutData("route1", "invalid doc")
	if err != nil {
		t.Error(err)
	}

	readClient, _ := createReadClient(&medium{typ: etcd, urls: urls, path: "/skippertest"})

	err = checkCmd(readClient, nil, nil, nil)
	if err != invalidRouteExpression {
		t.Error("failed to fail properly")
	}
}

func TestCheckEtcd(t *testing.T) {
	urls, err := stringsToUrls(etcdtest.Urls...)
	if err != nil {
		t.Error(err)
	}

	etcdtest.DeleteAll()
	etcdtest.PutData("route1", `Method("POST") -> <shunt>`)
	if err != nil {
		t.Error(err)
	}

	readClient, _ := createReadClient(&medium{typ: etcd, urls: urls, path: "/skippertest"})

	err = checkCmd(readClient, nil, nil, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestCheckDocInvalid(t *testing.T) {
	readClient, _ := createReadClient(&medium{typ: inline, eskip: "invalid doc"})

	err := checkCmd(readClient, nil, nil, nil)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestCheckDoc(t *testing.T) {
	readClient, _ := createReadClient(&medium{typ: inline, eskip: `Method("POST") -> <shunt>`})

	err := checkCmd(readClient, nil, nil, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestPatch(t *testing.T) {
	preserveOut := stdout

	for _, ti := range []struct {
		msg      string
		client   readClient
		media    []*medium
		err      bool
		expected []string
	}{{
		msg:    "no routes, no patch",
		client: testClient(""),
	}, {
		msg:      "with routes, no patch",
		client:   testClient(`r0: * -> <shunt>; r1: Method("GET") -> filter() -> "http://::1"`),
		expected: []string{`r0: * -> <shunt>;`, `r1: Method("GET") -> filter() -> "http://::1";`},
	}, {
		msg:    "invalid routes",
		client: testClient("not an eskip document"),
		err:    true,
	}, {
		msg:    "invalid patch",
		client: testClient("* -> <shunt>"),
		media:  []*medium{{typ: patchPrepend, patchFilters: "not an eskip expression"}},
		err:    true,
	}, {
		msg:    "prepend only",
		client: testClient(`r0: * -> <shunt>; r1: Method("GET") -> filter() -> "http://::1"`),
		media:  []*medium{{typ: patchPrepend, patchFilters: "filter0() -> filter1()"}},
		expected: []string{
			`r0: * -> filter0() -> filter1() -> <shunt>;`,
			`r1: Method("GET") -> filter0() -> filter1() -> filter() -> "http://::1";`},
	}, {
		msg:    "append only",
		client: testClient(`r0: * -> <shunt>; r1: Method("GET") -> filter() -> "http://::1"`),
		media:  []*medium{{typ: patchAppend, patchFilters: "filter0() -> filter1()"}},
		expected: []string{
			`r0: * -> filter0() -> filter1() -> <shunt>;`,
			`r1: Method("GET") -> filter() -> filter0() -> filter1() -> "http://::1";`},
	}, {
		msg:    "prepend and append",
		client: testClient(`r0: * -> <shunt>; r1: Method("GET") -> filter() -> "http://::1"`),
		media: []*medium{
			{typ: patchPrepend, patchFilters: "filter0p() -> filter1p()"},
			{typ: patchAppend, patchFilters: "filter0a() -> filter1a()"}},
		expected: []string{
			`r0: * -> filter0p() -> filter1p() -> filter0a() -> filter1a() -> <shunt>;`,
			`r1: Method("GET") -> filter0p() -> filter1p() -> filter() -> filter0a() -> filter1a() -> "http://::1";`},
	}} {
		buf := &bytes.Buffer{}
		var err error
		func() {
			defer func() { stdout = preserveOut }()
			stdout = buf
			err = patchCmd(ti.client, nil, nil, ti.media)
		}()

		if ti.err && err == nil {
			t.Error(ti.msg, "failed to fail")
			continue
		} else if !ti.err && err != nil {
			t.Error(ti.msg, "unexpected error", err)
			continue
		}

		if strings.TrimSpace(buf.String()) != strings.Join(ti.expected, "\n") {
			t.Error(ti.msg, "patch failed", buf.String())
		}
	}
}
