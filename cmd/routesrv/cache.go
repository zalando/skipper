package main

import (
	"bytes"
	"sync"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

// cache, once started, keeps an eskip-formatted copy of the most recent routes
// fetched using the provided data client.
type cache struct {
	client       routing.DataClient
	pollInterval time.Duration
	quit         chan struct{}

	formattedRoutes []byte
	mu              sync.RWMutex
}

func newCache(client routing.DataClient, pollInterval time.Duration) *cache {
	c := &cache{client: client, pollInterval: pollInterval}
	go c.poll()

	return c
}

func (c *cache) get() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.formattedRoutes
}

func (c *cache) formatAndSet(routes []*eskip.Route) {
	formattedRoutes := &bytes.Buffer{}
	eskip.Fprint(formattedRoutes, eskip.PrettyPrintInfo{}, routes...)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.formattedRoutes = formattedRoutes.Bytes()
}

func (c *cache) poll() {
	for {
		routes, err := c.client.LoadAll()
		if err != nil {
			// log.Error("failed to fetch routes: %s", err)
		} else {
			c.formatAndSet(routes)
		}

		select {
		case <-c.quit:
			return
		case <-time.After(c.pollInterval):
		}
	}
}
