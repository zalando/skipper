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

type readClient interface {
	LoadAndParseAll() ([]*eskip.RouteInfo, error)
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

func createReadClient(m *medium) (readClient, error) {
	// no output, no client
	if m == nil {
		return nil, nil
	}

	switch m.typ {
	case innkeeper:
		return createInnkeeperClient(m)

	case etcd:
		return etcdclient.New(etcdclient.Options{
			Endpoints: urlsToStrings(m.urls),
			Prefix:    m.path,
			Insecure:  insecure})

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

func createInnkeeperClient(m *medium) (*innkeeperclient.Client, error) {
	auth := innkeeperclient.CreateInnkeeperAuthentication(innkeeperclient.AuthOptions{InnkeeperAuthToken: m.oauthToken})

	ic, err := innkeeperclient.New(innkeeperclient.Options{
		Address:        m.urls[0].String(),
		Insecure:       false,
		Authentication: auth})

	if err != nil {
		return nil, err
	}
	return ic, nil
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

func (r *inlineReader) LoadAndParseAll() ([]*eskip.RouteInfo, error) {
	routes, err := eskip.Parse(r.routes)
	if err != nil {
		return nil, err
	}
	return routesToRouteInfos(routes), nil
}

func (r *idsReader) LoadAndParseAll() ([]*eskip.RouteInfo, error) {
	routeInfos := make([]*eskip.RouteInfo, len(r.ids))
	for i, id := range r.ids {
		routeInfos[i] = &eskip.RouteInfo{eskip.Route{Id: id}, nil}
	}

	return routeInfos, nil
}

func routesToRouteInfos(routes []*eskip.Route) (routeInfos []*eskip.RouteInfo) {
	for _, route := range routes {
		routeInfos = append(routeInfos, &eskip.RouteInfo{*route, nil})
	}
	return
}
