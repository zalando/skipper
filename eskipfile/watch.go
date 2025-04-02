package eskipfile

import (
	"bytes"
	"os"
	"reflect"
	"sync"

	"github.com/zalando/skipper/eskip"
)

type watchResponse struct {
	routes     []*eskip.Route
	deletedIDs []string
	err        error
}

// WatchClient implements a route configuration client with file watching. Use the Watch function to initialize
// instances of it.
type WatchClient struct {
	fileName    string
	lastContent []byte
	routes      map[string]*eskip.Route
	getAll      chan (chan<- watchResponse)
	getUpdates  chan (chan<- watchResponse)
	quit        chan struct{}
	once        sync.Once
}

// Watch creates a route configuration client with file watching. Watch doesn't follow file system nodes, it
// always reads from the file identified by the initially provided file name.
func Watch(name string) *WatchClient {
	c := &WatchClient{
		fileName:   name,
		getAll:     make(chan (chan<- watchResponse)),
		getUpdates: make(chan (chan<- watchResponse)),
		quit:       make(chan struct{}),
		once:       sync.Once{},
	}

	go c.watch()
	return c
}

func mapRoutes(r []*eskip.Route) map[string]*eskip.Route {
	m := make(map[string]*eskip.Route)
	for i := range r {
		m[r[i].Id] = r[i]
	}

	return m
}

func (c *WatchClient) storeRoutes(r []*eskip.Route) {
	c.routes = mapRoutes(r)
}

func (c *WatchClient) diffStoreRoutes(r []*eskip.Route) (upsert []*eskip.Route, deletedIDs []string) {
	for i := range r {
		if !reflect.DeepEqual(r[i], c.routes[r[i].Id]) {
			upsert = append(upsert, r[i])
		}
	}

	m := mapRoutes(r)
	for id := range c.routes {
		if _, keep := m[id]; !keep {
			deletedIDs = append(deletedIDs, id)
		}
	}

	c.routes = m
	return
}

func (c *WatchClient) deleteAllListIDs() []string {
	var ids []string
	for id := range c.routes {
		ids = append(ids, id)
	}

	c.routes = nil
	return ids
}

func cloneRoutes(r []*eskip.Route) []*eskip.Route {
	if len(r) == 0 {
		return nil
	}

	c := make([]*eskip.Route, len(r))
	for i, ri := range r {
		c[i] = ri.Copy()
	}

	return c
}

func (c *WatchClient) loadAll() watchResponse {
	content, err := os.ReadFile(c.fileName)
	if err != nil {
		c.lastContent = nil
		return watchResponse{err: err}
	}

	r, err := eskip.Parse(string(content))
	if err != nil {
		c.lastContent = nil
		return watchResponse{err: err}
	}

	c.storeRoutes(r)
	c.lastContent = content
	return watchResponse{routes: cloneRoutes(r)}
}

func (c *WatchClient) loadUpdates() watchResponse {
	content, err := os.ReadFile(c.fileName)
	if err != nil {
		c.lastContent = nil
		if os.IsNotExist(err) {
			deletedIDs := c.deleteAllListIDs()
			return watchResponse{deletedIDs: deletedIDs}
		}
		return watchResponse{err: err}
	}

	if bytes.Equal(content, c.lastContent) {
		return watchResponse{}
	}

	r, err := eskip.Parse(string(content))
	if err != nil {
		c.lastContent = nil
		return watchResponse{err: err}
	}

	upsert, del := c.diffStoreRoutes(r)
	c.lastContent = content
	return watchResponse{routes: cloneRoutes(upsert), deletedIDs: del}
}

func (c *WatchClient) watch() {
	for {
		select {
		case req := <-c.getAll:
			req <- c.loadAll()
		case req := <-c.getUpdates:
			req <- c.loadUpdates()
		case <-c.quit:
			return
		}
	}
}

// LoadAll returns the parsed route definitions found in the file.
func (c *WatchClient) LoadAll() ([]*eskip.Route, error) {
	req := make(chan watchResponse)
	select {
	case c.getAll <- req:
	case <-c.quit:
		return nil, nil
	}
	rsp := <-req
	return rsp.routes, rsp.err
}

// LoadUpdate returns differential updates when a watched file has changed.
func (c *WatchClient) LoadUpdate() ([]*eskip.Route, []string, error) {
	req := make(chan watchResponse)
	select {
	case c.getUpdates <- req:
	case <-c.quit:
		return nil, nil, nil
	}
	rsp := <-req
	return rsp.routes, rsp.deletedIDs, rsp.err
}

// Close stops watching the configured file and providing updates.
func (c *WatchClient) Close() {
	c.once.Do(func() {
		close(c.quit)
	})
}
