package eskipfile

import (
	"bytes"
	"os"
	"slices"
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

func (*WatchClient) Name() string {
	return "eskipfile-watch"
}

func mapRoutes(r []*eskip.Route) map[string]*eskip.Route {
	m := make(map[string]*eskip.Route, len(r))
	for i := range r {
		m[r[i].Id] = r[i]
	}

	return m
}

func (c *WatchClient) storeRoutes(r []*eskip.Route) {
	c.routes = mapRoutes(r)
}

func routesEqual(a, b *eskip.Route) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Id != b.Id ||
		a.Path != b.Path ||
		a.Method != b.Method ||
		a.Shunt != b.Shunt ||
		a.BackendType != b.BackendType ||
		a.Backend != b.Backend ||
		a.LBAlgorithm != b.LBAlgorithm ||
		a.Name != b.Name ||
		a.Namespace != b.Namespace {
		return false
	}
	if (a.HostRegexps == nil) != (b.HostRegexps == nil) || !slices.Equal(a.HostRegexps, b.HostRegexps) {
		return false
	}
	if (a.PathRegexps == nil) != (b.PathRegexps == nil) || !slices.Equal(a.PathRegexps, b.PathRegexps) {
		return false
	}
	if (a.Headers == nil) != (b.Headers == nil) || len(a.Headers) != len(b.Headers) {
		return false
	}
	for k, va := range a.Headers {
		if vb, ok := b.Headers[k]; !ok || va != vb {
			return false
		}
	}
	if (a.HeaderRegexps == nil) != (b.HeaderRegexps == nil) || len(a.HeaderRegexps) != len(b.HeaderRegexps) {
		return false
	}
	for k, va := range a.HeaderRegexps {
		vb, ok := b.HeaderRegexps[k]
		if !ok || (va == nil) != (vb == nil) || !slices.Equal(va, vb) {
			return false
		}
	}
	if (a.Predicates == nil) != (b.Predicates == nil) || len(a.Predicates) != len(b.Predicates) {
		return false
	}
	for i := range a.Predicates {
		pa, pb := a.Predicates[i], b.Predicates[i]
		if pa == pb {
			continue
		}
		if pa == nil || pb == nil || pa.Name != pb.Name ||
			(pa.Args == nil) != (pb.Args == nil) || !slices.Equal(pa.Args, pb.Args) {
			return false
		}
	}
	if (a.Filters == nil) != (b.Filters == nil) || len(a.Filters) != len(b.Filters) {
		return false
	}
	for i := range a.Filters {
		fa, fb := a.Filters[i], b.Filters[i]
		if fa == fb {
			continue
		}
		if fa == nil || fb == nil || fa.Name != fb.Name ||
			(fa.Args == nil) != (fb.Args == nil) || !slices.Equal(fa.Args, fb.Args) {
			return false
		}
	}
	if (a.LBEndpoints == nil) != (b.LBEndpoints == nil) || len(a.LBEndpoints) != len(b.LBEndpoints) {
		return false
	}
	for i := range a.LBEndpoints {
		ea, eb := a.LBEndpoints[i], b.LBEndpoints[i]
		if ea == eb {
			continue
		}
		if ea == nil || eb == nil || ea.Address != eb.Address || ea.Zone != eb.Zone {
			return false
		}
	}
	return true
}

func (c *WatchClient) diffStoreRoutes(r []*eskip.Route) (upsert []*eskip.Route, deletedIDs []string) {
	for i := range r {
		if !routesEqual(r[i], c.routes[r[i].Id]) {
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
