package main

import (
	"errors"
	etcdclient "github.com/coreos/go-etcd/etcd"
	"github.com/zalando/skipper/etcd/etcdtest"
	"log"
	"net/url"
	"os"
	"testing"
)

const testStdinName = "testStdin"

var ioError = errors.New("io error")

func parseUrls(surls []string) ([]*url.URL, error) {
	urls := make([]*url.URL, len(surls))
	for i, s := range surls {
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}

		urls[i] = u
	}

	return urls, nil
}

func deleteRoutesFrom(storageRoot string) {
	c := etcdclient.NewClient(etcdtest.Urls)
	c.Delete(storageRoot, true)
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
		err := checkCmd(&medium{typ: stdin}, nil)
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
		err := checkCmd(&medium{typ: stdin}, nil)
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
		err := checkCmd(&medium{typ: file, path: name}, nil)
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
		err := checkCmd(&medium{typ: file, path: name}, nil)
		if err != nil {
			t.Error(err)
		}
	})

	if err != nil {
		t.Error(err)
	}
}

func TestCheckEtcdInvalid(t *testing.T) {
	urls, err := parseUrls(etcdtest.Urls)
	if err != nil {
		t.Error(err)
	}

	deleteRoutes()
	c := etcdclient.NewClient(etcdtest.Urls)
	_, err = c.Set("/skippertest/routes/route1", "invalid doc", 0)
	if err != nil {
		t.Error(err)
	}

	err = checkCmd(&medium{typ: etcd, urls: urls, path: "/skippertest"}, nil)
	if err != invalidRouteExpression {
		t.Error("failed to fail properly")
	}
}

func TestCheckEtcd(t *testing.T) {
	urls, err := parseUrls(etcdtest.Urls)
	if err != nil {
		t.Error(err)
	}

	deleteRoutes()
	c := etcdclient.NewClient(etcdtest.Urls)
	_, err = c.Set("/skippertest/routes/route1", `Method("POST") -> <shunt>`, 0)
	if err != nil {
		t.Error(err)
	}

	err = checkCmd(&medium{typ: etcd, urls: urls, path: "/skippertest"}, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestCheckDocInvalid(t *testing.T) {
	err := checkCmd(&medium{typ: inline, eskip: "invalid doc"}, nil)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestCheckDoc(t *testing.T) {
	err := checkCmd(&medium{typ: inline, eskip: `Method("POST") -> <shunt>`}, nil)
	if err != nil {
		t.Error(err)
	}
}
