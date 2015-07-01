package etcd

import (
	"github.com/coreos/go-etcd/etcd"
	"log"
	"path"
	"skipper/skipper"
	"strings"
	"time"
)

const (
	idleEtcdWaitTimeShort   = 12 * time.Millisecond
	idleEtcdWaitTimeLong    = 6 * time.Second
	shortTermIdleRetryCount = 12
	routesPath              = "/routes"
)

type client struct {
	routesRoot string
	etcd       *etcd.Client
	push       chan skipper.RawData
}

type dataWrapper struct {
	data []string
}

// Creates a client receiving the configuraton from etcd. In the urls parameter it expects one or more valid urls to the
// supporting etcd service. In the storageRoot parameter it expects the root key for configuration, typically
// 'skipper' or 'skippertest'.
func Make(urls []string, storageRoot string) (skipper.DataClient, error) {
	c := &client{
		storageRoot + routesPath,
		etcd.NewClient(urls),
		make(chan skipper.RawData)}

	// parse and push the current data, then start waiting for updates, then repeat
	go func() {
		var (
			r               *etcd.Response
			data            []string
			etcdIndex uint64
			err             error
		)

		for {
			if r == nil {
				r = c.forceGet()
                etcdIndex = r.EtcdIndex
			} else {
				log.Println("watching for configuration update")
				r, err = c.watch(etcdIndex + 1)
                etcdIndex = r.EtcdIndex
				if err != nil {
					log.Println("error during watching for configuration update", err)
					log.Println("trying to get initial data")
					continue
				}
			}

			c.iterateRoutes(r.Node, &data)
			c.push <- &dataWrapper{data}
		}
	}()

	return c, nil
}

func getRouteId(r string) string {
	return r[:strings.Index(r, ":")]
}

// collect all the routes from the etcd nodes
func (c *client) iterateRoutes(n *etcd.Node, data *[]string) {
	if n.Key == c.routesRoot {
		for _, ni := range n.Nodes {
			c.iterateRoutes(ni, data)
		}
	}

	if path.Dir(n.Key) != c.routesRoot {
		return
	}

	rid := path.Base(n.Key)
	r := rid + ":" + n.Value

	existing := -1
	for i, ri := range *data {
		if getRouteId(ri) == rid {
			existing = i
			break
		}
	}

	if existing < 0 {
		*data = append(*data, r)
	} else {
		(*data)[existing] = r
	}
}

// get all settings
func (c *client) get() (*etcd.Response, error) {
	return c.etcd.Get(c.routesRoot, false, true)
}

// to ensure continuity when the etcd service may be out,
// we keep requesting the initial set of data with a timeout
// until it succeeds
func (c *client) forceGet() *etcd.Response {
	tryCount := uint(0)
	for {
		r, err := c.get()
		if err == nil {
			return r
		}

		log.Println("error during getting initial set of data", err)

		// to avoid too rapid retries, we put a small timeout here
		// for longer etcd outage, we increase the timeout after a fre tries
		to := idleEtcdWaitTimeShort
		if tryCount > shortTermIdleRetryCount {
			to = idleEtcdWaitTimeLong
		}

		time.Sleep(to)

		// ignore overflow, doesn't cause harm here
		tryCount = tryCount + 1
	}
}

// waits for updates in the etcd configuration
func (c *client) watch(waitIndex uint64) (*etcd.Response, error) {
	return c.etcd.Watch(c.routesRoot, waitIndex, true, nil, nil)
}

// Returns a channel that sends the the data on initial start, and on any update in the
// configuration. The channel blocks between two updates.
func (c *client) Receive() <-chan skipper.RawData {
	return c.push
}

// return the eskip representation of the current data
func (dw *dataWrapper) Get() string {
	return strings.Join(dw.data, ";")
}
