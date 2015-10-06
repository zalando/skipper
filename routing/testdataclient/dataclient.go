package testdataclient

import (
    "github.com/zalando/skipper/eskip"
    "github.com/zalando/skipper/routing"
    "log"
)

type T struct {}

func New() DataClient {
}

func (c *T) GetInitial() ([]*eskip.Route, error) {}

func (c *T) GetUpdate() ([]*eskip.Route, []string, error) {}
func (c *T) Update(upsert []*eskip.Route, deleteIds []string) {}
func (c *T) FailNext() {}
