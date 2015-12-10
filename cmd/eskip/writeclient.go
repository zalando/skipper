package main

import "github.com/zalando/skipper/eskip"
import (
innkeeperclient "github.com/zalando/skipper/innkeeper"
"errors"
	etcdclient "github.com/zalando/skipper/etcd"
)

type WriteClient interface {
	UpsertAll(routes []*eskip.Route) error
}

var (
	invalidOutput = errors.New("invalid output")
)

func createWriteClient(out *medium) (WriteClient, error) {
	// no out put, no client
	if out == nil {
		return nil, nil
	}

	switch out.typ {
	case innkeeper:
		auth := innkeeperclient.CreateInnkeeperAuthentication(innkeeperclient.AuthOptions{InnkeeperAuthToken:out.oauthToken})

		ic, err := innkeeperclient.New(innkeeperclient.Options{
			Address:        out.urls[0].String(),
			Insecure:       true,
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


