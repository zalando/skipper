package testdataclient

import (
    "github.com/zalando/skipper/eskip"
    "github.com/zalando/skipper/routing"
    "log"
)

type feed struct {
    insert string
    del []string
}

type TestDataClient struct {
    routes chan []*eskip.Route
    updates chan *routing.DataUpdate
    feed chan feed
}

func New(data string) *TestDataClient {
    dc := &TestDataClient{
        routes: make(chan []*eskip.Route),
        updates: make(chan *routing.DataUpdate),
        feed: make(chan feed)}

    go func () {
        routes := make(map[string]*eskip.Route)
        var routeList []*eskip.Route
        receive: for {
            select {
            case dc.routes <- routeList:
            case feed := <-dc.feed:
                newRoutes, err := eskip.Parse(feed.insert)
                if err != nil {
                    log.Println(err)
                    continue receive
                }

                for _, id := range feed.del {
                    delete(routes, id)
                }

                for _, r := range newRoutes {
                    routes[r.Id] = r
                }

                routeList = make([]*eskip.Route, len(routes))
                i := 0
                for _, r := range routes {
                    routeList[i] = r
                    i++
                }

                dc.updates <- &routing.DataUpdate{newRoutes, feed.del}
            }
        }
    }()

    dc.Feed(data, nil)
    <-dc.updates
    return dc
}

func (dc *TestDataClient) Receive() ([]*eskip.Route, <-chan *routing.DataUpdate) {
    return <-dc.routes, dc.updates
}

func (dc *TestDataClient) Feed(insertDoc string, deleteIds []string) {
    dc.feed <- feed{insertDoc, deleteIds}
}
