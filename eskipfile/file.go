package eskipfile

import (
	"github.com/zalando/skipper/eskip"
	"io/ioutil"
)

type Client string

func (c Client) GetInitial() ([]*eskip.Route, error) {
	content, err := ioutil.ReadFile(string(c))
	if err != nil {
		return nil, err
	}

	routes, err := eskip.Parse(string(content))
	if err != nil {
		return nil, err
	}

	return routes, nil
}

func (c Client) GetUpdate() ([]*eskip.Route, []string, error) { return nil, nil, nil }
