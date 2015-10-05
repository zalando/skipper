package etcd

import (
    "github.com/zalando/skipper/eskip"
    "github.com/zalando/skipper/routing"
	"github.com/coreos/go-etcd/etcd"
    "path"
    "strings"
)

const routesPath = "/routes"

type Client struct {
    routesRoot string
	etcd       *etcd.Client
}

func New(urls []string, storageRoot string) *Client {
    return &Client{
        storageRoot + routesPath,
		etcd.NewClient(urls)}
}

// collect all the routes from the etcd nodes
func (c *Client) iterateRoutes(n *etcd.Node, highestIndex uint64) ([]string, uint64) {
	if n.ModifiedIndex > highestIndex {
		highestIndex = n.ModifiedIndex
	}

    var routes []string
	if n.Key == c.routesRoot {
		for _, ni := range n.Nodes {
			rs, hi := c.iterateRoutes(ni, highestIndex)
            routes = append(routes, rs...)
            highestIndex = hi
		}
	}

	if path.Dir(n.Key) != c.routesRoot {
		return routes, highestIndex
	}

	rid := path.Base(n.Key)
	r := rid + ":" + n.Value
    return []string{r}, highestIndex
}

func (c *Client) Receive() ([]*eskip.Route, <-chan *routing.DataUpdate) {
	response, err := c.etcd.Get(c.routesRoot, false, true)
    if err != nil {
        println("etcd error", err)
        return nil, nil
    }

    data, _ := c.iterateRoutes(response.Node, 0)
    doc := strings.Join(data, ";")
    routes, err := eskip.Parse(doc)
    if err != nil {
        println(err.Error(), doc)
    }

    return routes, nil
}
