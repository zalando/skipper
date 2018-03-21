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
	"os"
	"strings"
	"testing"

	"github.com/zalando/skipper/etcd/etcdtest"
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
		err := checkCmd(cmdArgs{in: &medium{typ: stdin}})
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
		err := checkCmd(cmdArgs{in: &medium{typ: stdin}})
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
		err := checkCmd(cmdArgs{in: &medium{typ: file, path: name}})
		if err != nil {
			t.Error(err)
		}
	})

	if err != nil {
		t.Error(err)
	}
}

func TestCheckRepeatingRouteIds(t *testing.T) {
	const name = "testFile"
	err := withFile(name, `foo: Method("POST") -> "https://www.example1.org";foo: Method("POST") -> "https://www.example1.org";`, func(_ *os.File) {
		err := checkCmd(cmdArgs{in: &medium{typ: file, path: name}})
		if err == nil {
			t.Error("Expected an error for repeating route names")
		}
	})

	if err != nil {
		t.Error(err)
	}
}

func TestCheckEtcdInvalid(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	urls, err := stringsToUrls(etcdtest.Urls...)
	if err != nil {
		t.Error(err)
	}

	etcdtest.DeleteAll()
	etcdtest.PutData("route1", "invalid doc")
	if err != nil {
		t.Error(err)
	}

	err = checkCmd(cmdArgs{in: &medium{typ: etcd, urls: urls, path: "/skippertest"}})
	if err != invalidRouteExpression {
		t.Error("failed to fail properly")
	}
}

func TestCheckEtcd(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	urls, err := stringsToUrls(etcdtest.Urls...)
	if err != nil {
		t.Error(err)
	}

	etcdtest.DeleteAll()
	etcdtest.PutData("route1", `Method("POST") -> <shunt>`)
	if err != nil {
		t.Error(err)
	}

	err = checkCmd(cmdArgs{in: &medium{typ: etcd, urls: urls, path: "/skippertest"}})
	if err != nil {
		t.Error(err)
	}
}

func TestCheckDocInvalid(t *testing.T) {
	err := checkCmd(cmdArgs{in: &medium{typ: inline, eskip: "invalid doc"}})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestCheckDoc(t *testing.T) {
	err := checkCmd(cmdArgs{in: &medium{typ: inline, eskip: `Method("POST") -> <shunt>`}})
	if err != nil {
		t.Error(err)
	}
}

func TestPatch(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		media    []*medium
		err      bool
		expected []string
	}{{
		msg:   "no routes, no patch",
		media: []*medium{{typ: inline}},
	}, {
		msg: "with routes, no patch",
		media: []*medium{{
			typ:   inline,
			eskip: `r0: * -> <shunt>; r1: Method("GET") -> filter() -> "http://::1"`}},
		expected: []string{`r0: * -> <shunt>;`, `r1: Method("GET") -> filter() -> "http://::1";`},
	}, {
		msg: "invalid routes",
		media: []*medium{{
			typ:   inline,
			eskip: "not an eskip document"}},
		err: true,
	}, {
		msg: "invalid patch",
		media: []*medium{
			{typ: inline, eskip: "* -> <shunt>"},
			{typ: patchPrepend, patchFilters: "not an eskip expression"}},
		err: true,
	}, {
		msg: "prepend only",
		media: []*medium{
			{typ: inline, eskip: `r0: * -> <shunt>; r1: Method("GET") -> filter() -> "http://::1"`},
			{typ: patchPrepend, patchFilters: "filter0() -> filter1()"}},
		expected: []string{
			`r0: * -> filter0() -> filter1() -> <shunt>;`,
			`r1: Method("GET") -> filter0() -> filter1() -> filter() -> "http://::1";`},
	}, {
		msg: "append only",
		media: []*medium{
			{typ: inline, eskip: `r0: * -> <shunt>; r1: Method("GET") -> filter() -> "http://::1"`},
			{typ: patchAppend, patchFilters: "filter0() -> filter1()"}},
		expected: []string{
			`r0: * -> filter0() -> filter1() -> <shunt>;`,
			`r1: Method("GET") -> filter() -> filter0() -> filter1() -> "http://::1";`},
	}, {
		msg: "prepend and append",
		media: []*medium{
			{typ: inline, eskip: `r0: * -> <shunt>; r1: Method("GET") -> filter() -> "http://::1"`},
			{typ: patchPrepend, patchFilters: "filter0p() -> filter1p()"},
			{typ: patchAppend, patchFilters: "filter0a() -> filter1a()"}},
		expected: []string{
			`r0: * -> filter0p() -> filter1p() -> filter0a() -> filter1a() -> <shunt>;`,
			`r1: Method("GET") -> filter0p() -> filter1p() -> filter() -> filter0a() -> filter1a() -> "http://::1";`},
	}} {
		var in *medium
		for _, m := range ti.media {
			if m.typ == inline {
				in = m
			}
		}
		if in == nil {
			t.Error(ti.msg, "invalid test case, no input")
		}

		var err error
		preserveOut := stdout
		buf := &bytes.Buffer{}
		func() {
			defer func() { stdout = preserveOut }()
			stdout = buf
			err = patchCmd(cmdArgs{in: in, allMedia: ti.media})
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
