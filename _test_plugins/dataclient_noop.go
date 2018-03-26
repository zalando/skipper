package main

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

type DataClient string

func InitDataClient([]string) (routing.DataClient, error) {
	var dc DataClient = ""
	return dc, nil
}

func (dc DataClient) LoadAll() ([]*eskip.Route, error) {
	return eskip.Parse(string(dc))
}

func (dc DataClient) LoadUpdate() ([]*eskip.Route, []string, error) {
	return nil, nil, nil
}
