/*
Package testdataclient provides a test implementation for the DataClient
interface of the skipper/routing package.

It uses in-memory route definitions that are passed in on construction,
and can upserted/deleted programmatically.
*/
package testdataclient

import (
	"errors"
	"time"

	"github.com/zalando/skipper/eskip"
)

type incomingUpdate struct {
	upsert     []*eskip.Route
	deletedIDs []string
}

// DataClient implementation.
type Client struct {
	routes       map[string]*eskip.Route
	upsert       []*eskip.Route
	deletedIDs   []string
	failNext     int
	loadAllDelay time.Duration
	signalUpdate chan incomingUpdate
	quit         chan struct{}
}

// Creates a Client with an initial set of route definitions.
func New(initial []*eskip.Route) *Client {
	routes := make(map[string]*eskip.Route)
	for _, r := range initial {
		routes[r.Id] = r
	}

	return &Client{
		routes:       routes,
		signalUpdate: make(chan incomingUpdate),
		quit:         make(chan struct{}),
	}
}

// Creates a Client with an initial set of route definitions in eskip
// format. If parsing the eskip document fails, returns an error.
func NewDoc(doc string) (*Client, error) {
	routes, err := eskip.Parse(doc)
	if err != nil {
		return nil, err
	}

	return New(routes), nil
}

// Returns the initial/current set of route definitions.
func (c *Client) LoadAll() ([]*eskip.Route, error) {
	if c.failNext > 0 {
		c.upsert, c.deletedIDs = nil, nil
		c.failNext--
		return nil, errors.New("failed to get routes")
	}

	time.Sleep(c.loadAllDelay)

	routes := make([]*eskip.Route, 0, len(c.routes))
	for _, r := range c.routes {
		routes = append(routes, r)
	}
	return routes, nil
}

// Returns the route definitions upserted/deleted since the last call to
// LoadAll.
func (c *Client) LoadUpdate() ([]*eskip.Route, []string, error) {
	select {
	case update := <-c.signalUpdate:
		c.upsert, c.deletedIDs = update.upsert, update.deletedIDs
	case <-c.quit:
		return nil, nil, nil
	}

	for _, id := range c.deletedIDs {
		delete(c.routes, id)
	}

	for _, r := range c.upsert {
		c.routes[r.Id] = r
	}

	if c.failNext > 0 {
		c.upsert, c.deletedIDs = nil, nil
		c.failNext--
		return nil, nil, errors.New("failed to get routes")
	}

	var (
		u []*eskip.Route
		d []string
	)

	u, d, c.upsert, c.deletedIDs = c.upsert, c.deletedIDs, nil, nil
	return u, d, nil
}

// Updates the current set of routes with new/modified and deleted
// route definitions.
func (c *Client) Update(upsert []*eskip.Route, deletedIDs []string) {
	c.signalUpdate <- incomingUpdate{upsert, deletedIDs}
}

// Updates the current set of routes with new/modified and deleted
// route definitions in eskip format. In case the parsing of the
// document fails, it returns an error.
func (c *Client) UpdateDoc(upsertDoc string, deletedIDs []string) error {
	routes, err := eskip.Parse(upsertDoc)
	if err != nil {
		return err
	}

	c.Update(routes, deletedIDs)
	return nil
}

// Sets the Client to fail on the next call to LoadAll or LoadUpdate.
// Repeated call to FailNext will result the Client to fail as many
// times as it was called.
func (c *Client) FailNext() {
	c.failNext++
}

func (c *Client) WithLoadAllDelay(d time.Duration) {
	c.loadAllDelay = d
}

func (c *Client) Close() {
	close(c.quit)
}
