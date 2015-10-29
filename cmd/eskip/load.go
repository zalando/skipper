package main

import (
	"errors"
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/eskipfile"
	etcdclient "github.com/zalando/skipper/etcd"
	"io"
	"io/ioutil"
	"net/url"
	"os"
)

type routeList []*eskip.Route

type loadResult struct {
	routes      routeList
	parseErrors map[string]error
}

var invalidRouteExpression = errors.New("one or more invalid route expressions")

func mapRouteInfo(allInfo []*etcdclient.RouteInfo) loadResult {
	lr := loadResult{make(routeList, len(allInfo)), make(map[string]error)}
	for i, info := range allInfo {
		lr.routes[i] = &info.Route
		if info.ParseError != nil {
			lr.parseErrors[info.Id] = info.ParseError
		}
	}

	return lr
}

func urlStrings(urls []*url.URL) []string {
	surls := make([]string, len(urls))
	for i, u := range urls {
		surls[i] = u.String()
	}

	return surls
}

func loadReader(r io.Reader) (loadResult, error) {
	// this pretty much disables continuous piping,
	// but since the reset command first upserts all
	// and deletes the diff only after, it may not
	// even be consistent to do continous piping.
	// May change in the future.
	doc, err := ioutil.ReadAll(r)
	if err != nil {
		return loadResult{}, err
	}

	routes, err := eskip.Parse(string(doc))
	return loadResult{routes: routes}, err
}

func loadFile(path string) (loadResult, error) {
	client, err := eskipfile.Open(path)
	if err != nil {
		return loadResult{}, err
	}

	routes, err := client.LoadAll()
	return loadResult{routes: routes}, err
}

func loadEtcd(urls []*url.URL, storageRoot string) (loadResult, error) {
	client := etcdclient.New(urlStrings(urls), storageRoot)
	info, err := client.LoadAndParseAll()
	return mapRouteInfo(info), err
}

func loadString(doc string) (loadResult, error) {
	routes, err := eskip.Parse(doc)
	return loadResult{routes: routes}, err
}

func loadIds(ids []string) (loadResult, error) {
	routes := make(routeList, len(ids))
	for i, id := range ids {
		routes[i] = &eskip.Route{Id: id}
	}

	return loadResult{routes: routes}, nil
}

func loadRoutes(in *medium) (loadResult, error) {
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
		return loadIds(in.ids)
	default:
		return loadResult{}, invalidInputType
	}
}

func checkParseErrors(lr loadResult) error {
	if len(lr.parseErrors) == 0 {
		return nil
	}

	for id, perr := range lr.parseErrors {
		printStderr(id, perr)
	}

	return invalidRouteExpression
}

func loadRoutesChecked(m *medium) (routeList, error) {
	lr, err := loadRoutes(m)
	if err != nil {
		return nil, err
	}

	return lr.routes, checkParseErrors(lr)
}

func loadRoutesUnchecked(m *medium) routeList {
	lr, _ := loadRoutes(m)
	return lr.routes
}

func checkCmd(in, _ *medium) error {
	_, err := loadRoutesChecked(in)
	return err
}

func printCmd(in, _ *medium) error {
	lr, err := loadRoutes(in)
	if err != nil {
		return err
	}

	for _, r := range lr.routes {
		if perr, hasError := lr.parseErrors[r.Id]; hasError {
			printStderr(r.Id, perr)
		} else {
			fmt.Println(r.String())
		}
	}

	if len(lr.parseErrors) > 0 {
		return invalidRouteExpression
	}

	return nil
}
