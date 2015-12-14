package main

import (
	"errors"
	"github.com/zalando/skipper/eskip"
	etcdclient "github.com/zalando/skipper/etcd"
	innkeeperclient "github.com/zalando/skipper/innkeeper"
)

type writeClient interface {
	UpsertAll(routes []*eskip.Route) error
	// delete all items in 'routes' that fulfil 'cond'.
	DeleteAllIf(routes []*eskip.Route, cond eskip.RoutePredicate) error
}

var invalidOutput = errors.New("invalid output")

func createWriteClient(out *medium) (writeClient, error) {
	// no output, no client
	if out == nil {
		return nil, nil
	}

	switch out.typ {
	case innkeeper:
		auth := innkeeperclient.CreateInnkeeperAuthentication(innkeeperclient.AuthOptions{InnkeeperAuthToken: out.oauthToken})

		ic, err := innkeeperclient.New(innkeeperclient.Options{
			Address:        out.urls[0].String(),
			Insecure:       false,
			Authentication: auth})

		if err != nil {
			return nil, err
		}
		return ic, nil
	case etcd:
		client := etcdclient.New(urlsToStrings(out.urls), out.path)
		return client, nil
	}
	return nil, invalidOutput
}
