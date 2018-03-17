package eskipfile

import (
	"io/ioutil"
	"reflect"
	"os"

	"github.com/zalando/skipper/eskip"
)

type watchResponse struct {
	routes []*eskip.Route
	deletedIDs []string
	err error
}

type WatchClient struct {
	fileName string
	routes map[string]*eskip.Route
	getAll chan (chan<- watchResponse)
	getUpdates chan (chan<- watchResponse)
	quit chan struct {}
}

func Watch(name string) *WatchClient {
	c := &WatchClient{
		fileName: name,
		getAll: make(chan (chan<- watchResponse)),
		getUpdates: make(chan (chan<- watchResponse)),
		quit: make(chan struct{}),
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
	println("storing", len(r))
	c.routes = mapRoutes(r)
}

func (c *WatchClient) diffStoreRoutes(r []*eskip.Route) (upsert []*eskip.Route, deletedIDs []string) {
	println("diffing", len(r), len(c.routes))

	for i := range r {
		if !reflect.DeepEqual(r[i], c.routes[r[i].Id]) {
			upsert = append(upsert, r[i])
		} else {
			println("unchanged", r[i].Id)
		}
	}

	m := mapRoutes(r)
	for id := range c.routes {
		if _, keep := m[id]; !keep {
			deletedIDs = append(deletedIDs, id)
			println("deleted", id)
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
	println("loading all")
	content, err := ioutil.ReadFile(c.fileName)
	if err != nil {
		println("load error")
		return watchResponse{err: err}
	}

	r, err := eskip.Parse(string(content))
	if err != nil {
		println("parse error")
		return watchResponse{err: err}
	}

	println("success")
	c.storeRoutes(r)
	return watchResponse{routes: r}
}

func (c *WatchClient) loadUpdates() watchResponse {
	println("loading update")
	content, err := ioutil.ReadFile(c.fileName)
	if err != nil {
		if _, isPerr := err.(*os.PathError); isPerr {
			println("path error")
			deletedIDs := c.deleteAllListIDs()
			return watchResponse{deletedIDs: deletedIDs}
		}

		println("load error")
		return watchResponse{err: err}
	}

	r, err := eskip.Parse(string(content))
	if err != nil {
		println("parse error")
		return watchResponse{err: err}
	}

	upsert, del := c.diffStoreRoutes(r)
	println("update", len(upsert), len(del))
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

func (c *WatchClient) LoadAll() ([]*eskip.Route, error) {
	req := make(chan watchResponse)
	c.getAll <- req
	rsp := <-req
	return rsp.routes, rsp.err
}

func (c *WatchClient) LoadUpdate() ([]*eskip.Route, []string, error) {
	req := make(chan watchResponse)
	c.getUpdates <- req
	rsp := <-req
	return rsp.routes, rsp.deletedIDs, rsp.err
}

func (c *WatchClient) Close() {
	close(c.quit)
}
