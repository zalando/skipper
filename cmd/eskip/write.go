package main

import (
	"github.com/zalando/skipper/eskip"
	etcdclient "github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters/flowid"
	"regexp"
)

const randomIdLength = 16

type (
	routeCond     func(*eskip.Route) bool
	twoRoutesCond func(left, right *eskip.Route) bool
	routeMap      map[string]*eskip.Route
)

var routeIdRx = regexp.MustCompile("\\W")

func any(_ *eskip.Route) bool { return true }

func routeDiffers(left, right *eskip.Route) bool {
	return left.String() != right.String()
}

func mapRoutes(routes routeList) routeMap {
	m := make(routeMap)
	for _, r := range routes {
		m[r.Id] = r
	}

	return m
}

func ensureId(r *eskip.Route) error {
	if r.Id != "" {
		return nil
	}

	id, err := flowid.NewFlowId(randomIdLength)
	if err != nil {
		return err
	}

	id = routeIdRx.ReplaceAllString(id, "x")
	r.Id = "route" + id
	return nil
}

func upsertAll(routes routeList, m *medium) error {
	client := etcdclient.New(urlStrings(m.urls), m.path)
	for _, r := range routes {
		ensureId(r)
		err := client.Upsert(r)
		if err != nil {
			return err
		}
	}

	return nil
}

func takeDiff(ref routeList, routes routeList, altCond twoRoutesCond) routeList {
	mref := mapRoutes(ref)
	var diff routeList
	for _, r := range routes {
		if rr, exists := mref[r.Id]; !exists || altCond(rr, r) {
			diff = append(diff, r)
		}
	}

	return diff
}

func upsertDifferent(existing routeList, update routeList, m *medium) error {
	diff := takeDiff(existing, update, routeDiffers)
	return upsertAll(diff, m)
}

func deleteAllIf(routes routeList, m *medium, cond routeCond) error {
	client := etcdclient.New(urlStrings(m.urls), m.path)
	for _, r := range routes {
		if !cond(r) {
			continue
		}

		err := client.Delete(r.Id)
		if err != nil {
			return err
		}
	}

	return nil
}

func upsertCmd(in, out *medium) error {
	routes, err := loadRoutesChecked(in)
	if err != nil {
		return err
	}

	existing := loadRoutesUnchecked(out)
	return upsertDifferent(existing, routes, out)
}

func resetCmd(in, out *medium) error {
	routes, err := loadRoutesChecked(in)
	if err != nil {
		return err
	}

	existing := loadRoutesUnchecked(out)
	err = upsertDifferent(existing, routes, out)
	if err != nil {
		return err
	}

	rm := mapRoutes(routes)
	notSet := func(r *eskip.Route) bool {
		_, set := rm[r.Id]
		return !set
	}

	return deleteAllIf(existing, out, notSet)
}

func deleteCmd(in, out *medium) error {
	routes, err := loadRoutesChecked(in)
	if err != nil {
		return err
	}

	return deleteAllIf(routes, out, any)
}
