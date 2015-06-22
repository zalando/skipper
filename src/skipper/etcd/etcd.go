package etcd

import (
	"encoding/json"
	"github.com/coreos/go-etcd/etcd"
	"log"
	"skipper/skipper"
	"strings"
)

const StorageRoot = "/skipper"

const (
	backendPrefix    = "/skipper/backends/"
	frontendPrefix   = "/skipper/frontends/"
	filterSpecPrefix = "/skipper/filter-specs/"
)

type client struct {
	receive chan skipper.RawData
}

type dataWrapper struct {
	data map[string]interface{}
}

type etcdResponse struct {
	data            map[string]interface{}
	highestModIndex uint64
}

func setDataItem(data *map[string]interface{}, categoryIndex int, category string, node *etcd.Node) {
	// todo: what to do with parsing errors when already running
	var v interface{}
	err := json.Unmarshal([]byte(node.Value), &v)
	if err != nil {
		return
	}

	categoryMap, ok := (*data)[category].(map[string]interface{})
	if !ok {
		categoryMap = make(map[string]interface{})
		(*data)[category] = categoryMap
	}

	categoryMap[node.Key[categoryIndex:]] = v
}

func walkInNodes(data *map[string]interface{}, highestModIndex *uint64, node *etcd.Node) {
	if len(node.Nodes) > 0 {
		for _, n := range node.Nodes {
			walkInNodes(data, highestModIndex, n)
		}
	}

	switch {
	case strings.Index(node.Key, backendPrefix) == 0:
		setDataItem(data, len(backendPrefix), "backends", node)
	case strings.Index(node.Key, frontendPrefix) == 0:
		setDataItem(data, len(frontendPrefix), "frontends", node)
	case strings.Index(node.Key, filterSpecPrefix) == 0:
		setDataItem(data, len(filterSpecPrefix), "filter-specs", node)
	}

	if node.ModifiedIndex > *highestModIndex {
		*highestModIndex = node.ModifiedIndex
	}
}

func processResponse(data *map[string]interface{}, highestModIndex *uint64, r *etcd.Response) {
	if *data == nil {
		*data = make(map[string]interface{})
	}

	walkInNodes(data, highestModIndex, r.Node)
}

func watch(ec *etcd.Client, waitIndex uint64, receive chan *etcd.Response) {
	for {
		// we don't return error here after already started up
		r, err := ec.Watch(StorageRoot, waitIndex, true, nil, nil)
		if err != nil {
			log.Println("error during watching etcd", err)
			continue
		}

		receive <- r
		return
	}
}

func Make(urls []string) (skipper.DataClient, error) {
	var (
		c               *client
		ec              *etcd.Client
		data            map[string]interface{}
		highestModIndex uint64
		etcdReceive     chan *etcd.Response
	)

	c = &client{make(chan skipper.RawData)}
	ec = etcd.NewClient(urls)
	data = make(map[string]interface{})
	highestModIndex = 0
	etcdReceive = make(chan *etcd.Response)

	r, err := ec.Get(StorageRoot, false, true)
	if err != nil {
		return nil, err
	}

	processResponse(&data, &highestModIndex, r)
	go watch(ec, highestModIndex+1, etcdReceive)
	go func() {
		for {
			select {
			case r = <-etcdReceive:
				processResponse(&data, &highestModIndex, r)
				go watch(ec, highestModIndex+1, etcdReceive)
			case c.receive <- &dataWrapper{data}:
			}
		}
	}()

	return c, nil
}

func (c *client) Receive() <-chan skipper.RawData {
	return c.receive
}

func (dw *dataWrapper) Get() map[string]interface{} {
	return dw.data
}
