package main

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/eskipfile"
	etcdclient "github.com/zalando/skipper/etcd"
	innkeeperclient "github.com/zalando/skipper/innkeeper"
	"io"
	"io/ioutil"
	"os"
)

type ReadClient interface {
	LoadAndParseAll() ([]*eskip.RouteInfo, error)

	LoadUpdate() (eskip.RouteList, []string, error)
}

type stdinReader struct {
	reader io.Reader
}

type inlineReader struct {
	routes string
}

type idsReader struct {
	ids []string
}

// TODO eliminate duplicate code in write and read clients
func createReadClient(m *medium) (ReadClient, error) {
	// no out put, no client
	if m == nil {
		return nil, nil
	}

	switch m.typ {
	case innkeeper:
		auth := innkeeperclient.CreateInnkeeperAuthentication(innkeeperclient.AuthOptions{InnkeeperAuthToken: m.oauthToken})

		ic, err := innkeeperclient.New(innkeeperclient.Options{
			Address:        m.urls[0].String(),
			Insecure:       true,
			Authentication: auth})

		if err != nil {
			return nil, err
		}
		return ic, nil

	case etcd:
		client := etcdclient.New(urlsToStrings(m.urls), m.path)
		return client, nil

	case stdin:
		return &stdinReader{reader: os.Stdin}, nil

	case file:
		return eskipfile.Open(m.path)

	case inline:
		return &inlineReader{routes: m.eskip}, nil

	case inlineIds:
		return &idsReader{ids: m.ids}, nil

	default:
		return nil, invalidInputType
	}
}

func (r *stdinReader) LoadAndParseAll() ([]*eskip.RouteInfo, error) {
	// this pretty much disables continuous piping,
	// but since the reset command first upserts all
	// and deletes the diff only after, it may not
	// even be consistent to do continous piping.
	// May change in the future.
	doc, err := ioutil.ReadAll(r.reader)
	if err != nil {
		return nil, err
	}

	routes, err := eskip.Parse(string(doc))

	if err != nil {
		return nil, err
	}

	return routesToRouteInfos(routes), nil
}

func (r *stdinReader) LoadUpdate() (eskip.RouteList, []string, error) {
	return nil, nil, nil
}

func (r *inlineReader) LoadAndParseAll() ([]*eskip.RouteInfo, error) {
	routes, err := eskip.Parse(r.routes)
	if err != nil {
		return nil, err
	}
	return routesToRouteInfos(routes), nil
}

func (r *inlineReader) LoadUpdate() (eskip.RouteList, []string, error) {
	return nil, nil, nil
}

func (r *idsReader) LoadAndParseAll() ([]*eskip.RouteInfo, error) {
	routeInfos := make([]*eskip.RouteInfo, len(r.ids))
	for i, id := range r.ids {
		routeInfos[i] = &eskip.RouteInfo{eskip.Route{Id: id}, nil}
	}

	return routeInfos, nil
}

func (r *idsReader) LoadUpdate() (eskip.RouteList, []string, error) {
	return nil, nil, nil
}

func routesToRouteInfos(routes eskip.RouteList) (routeInfos []*eskip.RouteInfo) {
	for _, route := range routes {
		routeInfos = append(routeInfos, &eskip.RouteInfo{*route, nil})
	}
	return
}
