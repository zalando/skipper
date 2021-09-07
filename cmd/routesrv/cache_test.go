package main

import (
	"time"

	"github.com/zalando/skipper/eskip"
)

type testClient struct {
}

func (t *testClient) LoadAll() ([]*eskip.Route, error) {
	time.Sleep(2 * time.Second)

	return []*eskip.Route{
		{Id: "hello"},
	}, nil
}

func (t *testClient) LoadUpdate() ([]*eskip.Route, []string, error) {
	return []*eskip.Route{}, []string{}, nil
}
