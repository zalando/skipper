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

func Make(urls []string, storageRoot string) (skipper.DataClient, error) {
	c := &client{
		storageRoot,
		etcd.NewClient(urls),
		make(chan *etcd.Response),
		make(chan skipper.RawData)}

	data := make(map[string]interface{})
	highestModIndex := uint64(0)

	r, err := c.etcd.Get(storageRoot, false, true)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			c.walkInNodes(&data, &highestModIndex, r.Node)
			c.push <- &dataWrapper{data}
			go c.watch(highestModIndex + 1)
			r = <-c.receive
		}
	}()

	return c, nil
}

func (c *client) categoryPrefix(category string) string {
	return fmt.Sprintf("%s/%s/", c.storageRoot, category)
}

func (c *client) setIfMatch(category string, data *map[string]interface{}, node *etcd.Node) {
	prefix := c.categoryPrefix(category)
	if strings.Index(node.Key, prefix) == 0 {
		setDataItem(category, data, node.Key[len(prefix):], node)
	}
}

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

func (c *client) Receive() <-chan skipper.RawData {
	return c.push
}

func (dw *dataWrapper) Get() map[string]interface{} {
	return dw.data
}
