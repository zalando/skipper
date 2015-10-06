package routing

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"log"
	"net/url"
	"time"
)

type incomingType uint

const (
	incomingReset incomingType = iota
	incomingUpdate
)

type routeDefs map[string]*eskip.Route

type incomingData struct {
	typ            incomingType
	client         DataClient
	upsertedRoutes []*eskip.Route
	deletedIds     []string
}

func receiveInitial(c DataClient, pollTimeout time.Duration, out chan<- *incomingData) {
	// todo: raise retry timeout after a few tries
	for {
		routes, err := c.GetInitial()
		if err != nil {
			log.Println("error while receiveing initial data;", err)
			time.Sleep(pollTimeout)
			continue
		}

		out <- &incomingData{incomingReset, c, routes, nil}
		return
	}
}

func receiveUpdates(c DataClient, pollTimeout time.Duration, out chan<- *incomingData) {
	for {
		time.Sleep(pollTimeout)
		routes, deletedIds, err := c.GetUpdate()
		if err != nil {
			log.Println("error while receiving update;", err)
			return
		}

		out <- &incomingData{incomingUpdate, c, routes, deletedIds}
	}
}

func receiveFromClient(c DataClient, pollTimeout time.Duration, out chan<- *incomingData) {
	for {
		receiveInitial(c, pollTimeout, out)
		receiveUpdates(c, pollTimeout, out)
	}
}

func applyIncoming(defs routeDefs, d *incomingData) routeDefs {
	if d.typ == incomingReset || defs == nil {
		defs = make(routeDefs)
	}

	if d.typ == incomingUpdate {
		for _, id := range d.deletedIds {
			delete(defs, id)
		}
	}

	if d.typ == incomingReset || d.typ == incomingUpdate {
		for _, def := range d.upsertedRoutes {
			defs[def.Id] = def
		}
	}

	return defs
}

func mergeDefs(defsByClient map[DataClient]routeDefs) []*eskip.Route {
	mergeById := make(routeDefs)
	for _, defs := range defsByClient {
		for id, def := range defs {
			mergeById[id] = def
		}
	}

	var all []*eskip.Route
	for _, def := range mergeById {
		all = append(all, def)
	}

	return all
}

func receiveRouteDefs(o Options) <-chan []*eskip.Route {
	in := make(chan *incomingData)
	out := make(chan []*eskip.Route)
	defsByClient := make(map[DataClient]routeDefs)

	for _, c := range o.DataClients {
		go receiveFromClient(c, o.PollTimeout, in)
	}

	go func() {
		for {
			incoming := <-in
			c := incoming.client
			defsByClient[c] = applyIncoming(defsByClient[c], incoming)
			out <- mergeDefs(defsByClient)
		}
	}()

	return out
}

func splitBackend(r *eskip.Route) (string, string, error) {
	if r.Shunt {
		return "", "", nil
	}

	bu, err := url.ParseRequestURI(r.Backend)
	if err != nil {
		return "", "", err
	}

	return bu.Scheme, bu.Host, nil
}

func createFilter(fr filters.Registry, def *eskip.Filter) (filters.Filter, error) {
	spec, ok := fr[def.Name]
	if !ok {
		return nil, fmt.Errorf("filter not found: '%s'", def.Name)
	}

	return spec.CreateFilter(def.Args)
}

func createFilters(fr filters.Registry, defs []*eskip.Filter) ([]filters.Filter, error) {
	var fs []filters.Filter
	for _, def := range defs {
		f, err := createFilter(fr, def)
		if err != nil {
			return nil, err
		}

		fs = append(fs, f)
	}

	return fs, nil
}

func processRouteDef(fr filters.Registry, def *eskip.Route) (*Route, error) {
	scheme, host, err := splitBackend(def)
	if err != nil {
		return nil, err
	}

	fs, err := createFilters(fr, def.Filters)
	if err != nil {
		return nil, err
	}

	return &Route{*def, scheme, host, fs}, nil
}

func processRouteDefs(fr filters.Registry, defs []*eskip.Route) []*Route {
	var routes []*Route
	for _, def := range defs {
		route, err := processRouteDef(fr, def)
		if err == nil {
			routes = append(routes, route)
		} else {
			log.Println(err)
		}
	}

	return routes
}

func receiveRouteMatcher(o Options, out chan<- *matcher) {
	updates := receiveRouteDefs(o)
	for {
		defs := <-updates
		routes := processRouteDefs(o.FilterRegistry, defs)
		m, errs := newMatcher(routes, o.MatchingOptions)
		for _, err := range errs {
			log.Println(err)
		}

		out <- m
	}
}
