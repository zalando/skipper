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
	etcdclient "github.com/coreos/go-etcd/etcd"
	"github.com/zalando/skipper/etcd/etcdtest"
	"log"
	"os"
	"testing"
)

const testStdinName = "testStdin"

var ioError = errors.New("io error")

func deleteRoutesFrom(prefix string) {
	c := etcdclient.NewClient(etcdtest.Urls)
	c.Delete(prefix, true)
}

func deleteRoutes() {
	deleteRoutesFrom("/skippertest")
}

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
		err := checkCmd(&medium{typ: stdin}, nil, nil)
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
		err := checkCmd(&medium{typ: stdin}, nil, nil)
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
		err := checkCmd(&medium{typ: file, path: name}, nil, nil)
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
		err := checkCmd(&medium{typ: file, path: name}, nil, nil)
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

	deleteRoutes()
	c := etcdclient.NewClient(etcdtest.Urls)
	_, err = c.Set("/skippertest/routes/route1", "invalid doc", 0)
	if err != nil {
		t.Error(err)
	}

	err = checkCmd(&medium{typ: etcd, urls: urls, path: "/skippertest"}, nil, nil)
	if err != invalidRouteExpression {
		t.Error("failed to fail properly")
	}
}

func TestCheckEtcd(t *testing.T) {
	urls, err := stringsToUrls(etcdtest.Urls...)
	if err != nil {
		t.Error(err)
	}

	deleteRoutes()
	c := etcdclient.NewClient(etcdtest.Urls)
	_, err = c.Set("/skippertest/routes/route1", `Method("POST") -> <shunt>`, 0)
	if err != nil {
		t.Error(err)
	}

	err = checkCmd(&medium{typ: etcd, urls: urls, path: "/skippertest"}, nil, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestCheckDocInvalid(t *testing.T) {
	err := checkCmd(&medium{typ: inline, eskip: "invalid doc"}, nil, nil)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestCheckDoc(t *testing.T) {
	err := checkCmd(&medium{typ: inline, eskip: `Method("POST") -> <shunt>`}, nil, nil)
	if err != nil {
		t.Error(err)
	}
}
