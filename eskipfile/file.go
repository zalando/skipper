package eskipfile

import (
	"github.com/zalando/skipper/eskip"
	"io/ioutil"
)

type Client struct{ routes []*eskip.Route }

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

func (c Client) GetInitial() ([]*eskip.Route, error)          { return c.routes, nil }
func (c Client) GetUpdate() ([]*eskip.Route, []string, error) { return nil, nil, nil }
