package eskipfile

import (
	"io/ioutil"
	"os"
	"reflect"

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
	fileName   string
	routes     map[string]*eskip.Route
	getAll     chan (chan<- watchResponse)
	getUpdates chan (chan<- watchResponse)
	quit       chan struct{}
}

// Watch creates a route configuration client with file watching. Watch doesn't follow file system nodes, it
// always reads from the file identified by the initially provided file name.
func Watch(name string) *WatchClient {
	c := &WatchClient{
		fileName:   name,
		getAll:     make(chan (chan<- watchResponse)),
		getUpdates: make(chan (chan<- watchResponse)),
		quit:       make(chan struct{}),
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

func (c *WatchClient) loadAll() watchResponse {
	content, err := ioutil.ReadFile(c.fileName)
	if err != nil {
		return watchResponse{err: err}
	}

	r, err := eskip.Parse(string(content))
	if err != nil {
		return watchResponse{err: err}
	}

	c.storeRoutes(r)
	return watchResponse{routes: r}
}

func (c *WatchClient) loadUpdates() watchResponse {
	content, err := ioutil.ReadFile(c.fileName)
	if err != nil {
		if _, isPerr := err.(*os.PathError); isPerr {
			deletedIDs := c.deleteAllListIDs()
			return watchResponse{deletedIDs: deletedIDs}
		}

		return watchResponse{err: err}
	}

	r, err := eskip.Parse(string(content))
	if err != nil {
		return watchResponse{err: err}
	}

	upsert, del := c.diffStoreRoutes(r)
	return watchResponse{routes: upsert, deletedIDs: del}
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
	c.getAll <- req
	rsp := <-req
	return rsp.routes, rsp.err
}

// LoadUpdate returns differential updates when a watched file has changed.
func (c *WatchClient) LoadUpdate() ([]*eskip.Route, []string, error) {
	req := make(chan watchResponse)
	c.getUpdates <- req
	rsp := <-req
	return rsp.routes, rsp.deletedIDs, rsp.err
}

// Close stops watching the configured file and providing updates.
func (c *WatchClient) Close() {
	close(c.quit)
}
