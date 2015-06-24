package etcd

import (
	"encoding/json"
	"fmt"
	"github.com/coreos/go-etcd/etcd"
	"log"
	"skipper/skipper"
	"strings"
)

type client struct {
	storageRoot string
	etcd        *etcd.Client
	receive     chan *etcd.Response
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
		make(chan *etcd.Response),
		make(chan skipper.RawData)}

	data := make(map[string]interface{})

	r, err := c.etcd.Get(storageRoot, false, true)
	if err != nil {
		return nil, err
	}

	// parse and push the current data, then start waiting for updates, then repeat
	go func() {
		for {
			c.walkInNodes(&data, r.Node)
			c.push <- &dataWrapper{data}
			go c.watch(r.EtcdIndex + 1)
			r = <-c.receive
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
func (c *client) walkInNodes(data *map[string]interface{}, node *etcd.Node) {
	if len(node.Nodes) > 0 {
		for _, n := range node.Nodes {
			c.walkInNodes(data, n)
		}
	}

	c.setIfMatch("backends", data, node)
	c.setIfMatch("frontends", data, node)
	c.setIfMatch("filter-specs", data, node)
}

// waits for updates in the etcd configuration
func (c *client) watch(waitIndex uint64) {
	for {
		// we don't return error here after already started up
		r, err := c.etcd.Watch(c.storageRoot, waitIndex, true, nil, nil)
		if err != nil {
			log.Println("error during watching etcd", err)
			continue
		}

		c.receive <- r
		return
	}
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
