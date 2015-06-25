package etcd

import (
	"encoding/json"
	"fmt"
	"github.com/coreos/go-etcd/etcd"
	"log"
	"skipper/skipper"
	"strings"
	"time"
)

const (
	idleEtcdWaitTimeShort   = 12 * time.Millisecond
	idleEtcdWaitTimeLong    = 6 * time.Second
	shortTermIdleRetryCount = 12
)

type client struct {
	storageRoot string
	etcd        *etcd.Client
	push        chan skipper.RawData
}

type dataWrapper struct {
	data map[string]interface{}
}

// sets the data for a given category and key from an etcd response node, where the category can be 'backends',
// 'frontends' or 'filter-specs'.
func setDataItem(category string, data *map[string]interface{}, key string, node *etcd.Node) {
	// todo: what to do with parsing errors when already running
	var v interface{}
	err := json.Unmarshal([]byte(node.Value), &v)
	if err != nil {
		log.Println(err)
		return
	}

	categoryMap, ok := (*data)[category].(map[string]interface{})
	if !ok {
		categoryMap = make(map[string]interface{})
		(*data)[category] = categoryMap
	}

	categoryMap[key] = v
}

// Creates a client receiving the configuraton from etcd. In the urls parameter it expects one or more valid urls to the
// supporting etcd service. In the storageRoot parameter it expects the root key for configuration, typically
// 'skipper' or 'skippertest'.
func Make(urls []string, storageRoot string) (skipper.DataClient, error) {
	c := &client{
		storageRoot,
		etcd.NewClient(urls),
		make(chan skipper.RawData)}

	data := make(map[string]interface{})

	// parse and push the current data, then start waiting for updates, then repeat
	go func() {
		var (
			r               *etcd.Response
			highestModIndex uint64
			err             error
		)

		for {
			if r == nil {
				r = c.forceGet()
			} else {
				log.Println("watching for configuration update")
				r, err = c.watch(highestModIndex + 1)
				if err != nil {
					log.Println("error during watching for configuration update", err)
					log.Println("trying to get initial data")
					continue
				}
			}

			c.walkInNodes(&data, &highestModIndex, r.Node)
			c.push <- &dataWrapper{data}
		}
	}()

	return c, nil
}

// create an etcd key prefix from a category, e.g. '/skipper/frontends'
func (c *client) categoryPrefix(category string) string {
	return fmt.Sprintf("%s/%s/", c.storageRoot, category)
}

// set an item for a given node in the current data map, if its key matches
func (c *client) setIfMatch(category string, data *map[string]interface{}, node *etcd.Node) {
	prefix := c.categoryPrefix(category)
	if strings.Index(node.Key, prefix) == 0 {
		setDataItem(category, data, node.Key[len(prefix):], node)
	}
}

// collects the changed nodes and updates the current data, regardless how deep subtree
// was returned from etcd
func (c *client) walkInNodes(data *map[string]interface{}, highestModIndex *uint64, node *etcd.Node) {
	if len(node.Nodes) > 0 {
		for _, n := range node.Nodes {
			c.walkInNodes(data, highestModIndex, n)
		}
	}

	c.setIfMatch("backends", data, node)
	c.setIfMatch("frontends", data, node)
	c.setIfMatch("filter-specs", data, node)

	if node.ModifiedIndex > *highestModIndex {
		*highestModIndex = node.ModifiedIndex
	}
}

func (c *client) get() (*etcd.Response, error) {
	return c.etcd.Get(c.storageRoot, false, true)
}

// to ensure continuity when the etcd service may be out,
// we keep requesting the initial set of data until it
// succeeds
func (c *client) forceGet() *etcd.Response {
	tryCount := 0
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

		tryCount = tryCount + 1
	}
}

// waits for updates in the etcd configuration
func (c *client) watch(waitIndex uint64) (*etcd.Response, error) {
	return c.etcd.Watch(c.storageRoot, waitIndex, true, nil, nil)
}

// Returns a channel that sends the the data on initial start, and on any update in the
// configuration. The channel blocks between two updates.
func (c *client) Receive() <-chan skipper.RawData {
	return c.push
}

// return the json-like representation of the current data
func (dw *dataWrapper) Get() map[string]interface{} {
	return dw.data
}
