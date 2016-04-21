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
	"github.com/zalando/skipper/etcd/etcdtest"
	"os"
	"testing"
)

const testStdinName = "testStdin"

var ioError = errors.New("io error")

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

		err := checkCmd(readClient, nil, nil)
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
		err := checkCmd(readClient, nil, nil)
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
		err := checkCmd(readClient, nil, nil)
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

	err = checkCmd(readClient, nil, nil)
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

	err = checkCmd(readClient, nil, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestCheckDocInvalid(t *testing.T) {
	readClient, _ := createReadClient(&medium{typ: inline, eskip: "invalid doc"})

	err := checkCmd(readClient, nil, nil)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestCheckDoc(t *testing.T) {
	readClient, _ := createReadClient(&medium{typ: inline, eskip: `Method("POST") -> <shunt>`})

	err := checkCmd(readClient, nil, nil)
	if err != nil {
		t.Error(err)
	}
}
