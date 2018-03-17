package eskipfile

import (
	"io/ioutil"

	"github.com/zalando/skipper/eskip"
)

// A Client contains the route definitions from an eskip file.
type Client struct{ routes []*eskip.Route }

// Opens an eskip file and parses it, returning a DataClient implementation.
// If reading or parsing the file fails, returns an error.
func Open(path string) (*Client, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	routes, err := eskip.Parse(string(content))
	if err != nil {
		return nil, err
	}

	return &Client{routes}, nil
}

func (c Client) LoadAndParseAll() (routeInfos []*eskip.RouteInfo, err error) {
	for _, route := range c.routes {
		routeInfos = append(routeInfos, &eskip.RouteInfo{Route: *route})
	}
	return
}

// Returns the parsed route definitions found in the file.
func (c Client) LoadAll() ([]*eskip.Route, error) { return c.routes, nil }

// Noop. The current implementation doesn't support watching the eskip
// file for changes.
func (c Client) LoadUpdate() ([]*eskip.Route, []string, error) { return nil, nil, nil }
