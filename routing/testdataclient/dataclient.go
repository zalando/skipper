package testdataclient

import (
	"errors"
	"github.com/zalando/skipper/eskip"
)

type C struct {
	routes       map[string]*eskip.Route
	upsert       []*eskip.Route
	deletedIds   []string
	failNext     int
	signalUpdate chan int
}

func New(initial []*eskip.Route) *C {
	routes := make(map[string]*eskip.Route)
	for _, r := range initial {
		routes[r.Id] = r
	}

	return &C{
		routes:       routes,
		signalUpdate: make(chan int)}
}

func (c *C) GetInitial() ([]*eskip.Route, error) {
	if c.failNext > 0 {
		c.upsert, c.deletedIds = nil, nil
		c.failNext--
		return nil, errors.New("failed to get routes")
	}

	var routes []*eskip.Route
	for _, r := range c.routes {
		routes = append(routes, r)
	}

	return routes, nil
}

func (c *C) GetUpdate() ([]*eskip.Route, []string, error) {
	<-c.signalUpdate

	for _, id := range c.deletedIds {
		delete(c.routes, id)
	}

	for _, r := range c.upsert {
		c.routes[r.Id] = r
	}

	if c.failNext > 0 {
		c.upsert, c.deletedIds = nil, nil
		c.failNext--
		return nil, nil, errors.New("failed to get routes")
	}

	var (
		u []*eskip.Route
		d []string
	)

	u, d, c.upsert, c.deletedIds = c.upsert, c.deletedIds, nil, nil
	return u, d, nil
}

func (c *C) Update(upsert []*eskip.Route, deletedIds []string) {
	c.upsert, c.deletedIds = upsert, deletedIds
	c.signalUpdate <- 42
}

func (c *C) FailNext() {
	c.failNext++
}
