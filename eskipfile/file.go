package eskipfile

import (
	"os"

	"github.com/zalando/skipper/eskip"
)

// Client contains the route definitions from an eskip file, if you
// need file watch use WatchClient instead.
//
// Use the Open function to create instances of Client.
type Client struct{ routes []*eskip.Route }

// Open opens an eskip file and parses it, returning a DataClient implementation. If reading or parsing the file
// fails, returns an error. This implementation doesn't provide file watch.
func Open(path string) (*Client, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	routes, err := eskip.Parse(string(content))
	if err != nil {
		return nil, err
	}

	return &Client{routes}, nil
}

func (Client) Name() string {
	return "eskipfile"
}

func (c Client) LoadAndParseAll() (routeInfos []*eskip.RouteInfo, err error) {
	for _, route := range c.routes {
		routeInfos = append(routeInfos, &eskip.RouteInfo{Route: *route})
	}
	return
}

// LoadAll returns the parsed route definitions found in the file.
func (c Client) LoadAll() ([]*eskip.Route, error) { return c.routes, nil }

// LoadUpdate is a noop.
func (c Client) LoadUpdate() ([]*eskip.Route, []string, error) { return nil, nil, nil }
