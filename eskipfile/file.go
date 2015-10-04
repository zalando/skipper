package eskipfile

import (
	"io/ioutil"
    "github.com/zalando/skipper/eskip"
    "github.com/zalando/skipper/routing"
)

type DataClient struct {
    data []*eskip.Route
    c chan *routing.DataUpdate
}

func New(path string) (*DataClient, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

    routes, err := eskip.Parse(string(content))
    if err != nil {
        return nil, err
    }

    c := make(chan *routing.DataUpdate)
    return &DataClient{routes, c}, nil
}

func (dc *DataClient) Receive() ([]*eskip.Route, <-chan *routing.DataUpdate) {
    return dc.data, dc.c
}
