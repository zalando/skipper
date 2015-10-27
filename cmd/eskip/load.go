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

func urlsString(urls []*url.URL) []string {
	surls := make([]string, len(urls))
	for i, u := range urls {
		surls[i] = u.String()
	}

	return surls
}

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

	return client.LoadAll()
}

func loadEtcd(urls []*url.URL, storageRoot string) ([]*eskip.Route, error) {
	// should return the invalid routes, too
	client := etcdclient.New(urlsString(urls), storageRoot)
	return client.LoadAll()
}

func loadString(doc string) ([]*eskip.Route, error) {
	return eskip.Parse(doc)
}

func loadIds(ids []string) []*eskip.Route {
	routes := make([]*eskip.Route, len(ids))
	for i, id := range ids {
		routes[i] = &eskip.Route{Id: id}
	}

	return routes
}

func loadRoutes(in *medium) ([]*eskip.Route, error) {
	switch in.typ {
	case stdin:
		return loadReader(os.Stdin)
	case file:
		return loadFile(in.path)
	case etcd:
		return loadEtcd(in.urls, in.path)
	case inline:
		return loadString(in.eskip)
	case inlineIds:
		return loadIds(in.ids), nil
	default:
		return nil, invalidInputType
	}
}

func printRoutes(routes []*eskip.Route) error {
	_, err := fmt.Println(eskip.String(routes...))
	return err
}

func checkCmd(in, _ *medium) error {
	_, err := loadRoutes(in)
	return err
}

func printCmd(in, _ *medium) error {
	routes, err := loadRoutes(in)
	if err != nil {
		return err
	}

	return printRoutes(routes)
}
