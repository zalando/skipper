package main

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/eskipfile"
	etcdclient "github.com/zalando/skipper/etcd"
	"io"
	"io/ioutil"
	"net/url"
	"os"
)

func loadReader(r io.Reader) ([]*eskip.Route, error) {

	// this pretty much disables continuous piping,
	// but since the reset command first upserts all
	// and deletes the diff only after, it may not
	// even be consistent to do continous streaming.
	// May change in the future.
	doc, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return eskip.Parse(string(doc))
}

func loadFile(path string) ([]*eskip.Route, error) {
	client, err := eskipfile.Open(path)
	if err != nil {
		return nil, err
	}

	return client.GetInitial()
}

func loadEtcd(urls []*url.URL, storageRoot string) ([]*eskip.Route, error) {
	surls := make([]string, len(urls))
	for i, u := range urls {
		surls[i] = u.String()
	}

	client := etcdclient.New(surls, storageRoot)
	return client.GetInitial()
}

func loadString(doc string) ([]*eskip.Route, error) {
	return eskip.Parse(doc)
}

func checkRoutes(in *medium) ([]*eskip.Route, error) {
	switch in.typ {
	case stdin:
		return loadReader(os.Stdin)
	case file:
		return loadFile(in.path)
	case etcd:
		return loadEtcd(in.urls, in.path)
	case inline:
		return loadString(in.eskip)
	default:
		return nil, invalidInputType
	}
}

func printRoutes(routes []*eskip.Route, out *medium) error {
	_, err := fmt.Println(eskip.String(routes...))
	return err
}

func checkCmd(in, _ *medium) error {
	_, err := checkRoutes(in)
	return err
}

func printCmd(in, out *medium) error {
	routes, err := checkRoutes(in)
	if err != nil {
		return err
	}

	return printRoutes(routes, out)
}
