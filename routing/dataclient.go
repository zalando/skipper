package routing

import (
    "github.com/zalando/skipper/eskip"
    "log"
    "time"
)

const (
    retryTimeout = 900 * time.Millisecond
	receiveInitialTimeout = 1200 * time.Millisecond
)

type DataUpdate struct {
    UpsertedRoutes []*eskip.Route
    DeletedIds []string
    Reset bool
}

type DataClient interface {
    Receive() ([]*eskip.Route, <-chan *DataUpdate)
}

type PollingSource interface {
    GetInitial() ([]*eskip.Route, error)
    GetUpdate() ([]*eskip.Route, []string, error)
}

type pollingDataClient struct {
    source PollingSource
    initial chan []*eskip.Route
    updates chan *DataUpdate
}

func (c *pollingDataClient) receiveInitial(initial bool) {
    for {
        routes, err := c.source.GetInitial()
        if err != nil {
            initial = false
            log.Println("error while receiveing initial data", err)
            time.Sleep(retryTimeout)
            continue
        }

        if initial {
            c.initial <- routes
        } else {
            c.updates <- &DataUpdate{routes, nil, true}
        }

        return
    }
}

func (c *pollingDataClient) receiveUpdates() {
    for {
        routes, deletedIds, err := c.source.GetUpdate()
        if err != nil {
            log.Println("error while receiving update", err)
            return
        }

        c.updates <- &DataUpdate{routes, deletedIds, false}
    }
}

func NewPollingDataClient(p PollingSource) DataClient {
    c := &pollingDataClient{
        p,
        make(chan []*eskip.Route),
        make(chan *DataUpdate)}

    go func() {
        c.receiveInitial(true)
        for {
            c.receiveUpdates()
            c.receiveInitial(false)
        }
    }()

    return c
}

func (c *pollingDataClient) Receive() ([]*eskip.Route, <-chan *DataUpdate) {
	var routes []*eskip.Route
	select {
	case routes = <-c.initial:
	case <-time.After(receiveInitialTimeout):
		log.Println("timeout while receiving initial set of routes")
	}

    return routes, c.updates
}
