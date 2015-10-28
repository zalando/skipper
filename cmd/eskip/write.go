package main

import (
	"errors"
	"github.com/zalando/skipper/eskip"
	etcdclient "github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters/flowid"
	"regexp"
)

const randomIdLength = 16

var (
	failedToGenerateId = errors.New("failed to generate id")
	routeIdRx          = regexp.MustCompile("\\W")
)

func ensureId(r *eskip.Route) {
    if r.Id != "" {
        return
    }

    id, err := flowid.NewFlowId(randomIdLength)
    if err != nil {
        return nil, failedToGenerateId
    }

    id = routeIdRx.ReplaceAllString(id, "x")
    r.Id = "route" + id
}

func upsertAll(routes []*eskip.Route, out *medium) (map[string]bool, error) {
	upserted := map[string]bool{}
	client := etcdclient.New(urlsString(out.urls), out.path)
	for _, r := range routes {
        ensureId(r)

		err := client.Upsert(r)
		if err != nil {
			return nil, err
		}

		upserted[r.Id] = true
	}

	return upserted, nil
}

func deleteAllIf(routes []*eskip.Route, out *medium, cond func(*eskip.Route) bool) error {
	client := etcdclient.New(urlsString(out.urls), out.path)
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
	routes, err := loadRoutes(in)
	if err != nil {
		return err
	}

	// TODO: to preserve performance, it should only update what differs or new
	_, err = upsertAll(routes, out)
	return err
}

func resetCmd(in, out *medium) error {
	routes, err := loadRoutes(in)
	if err != nil {
		return err
	}

	// TODO:
	// should get all the ids, regardless if it is valid, and
	// then use the error if any
	existing, _ := loadRoutes(out)

	upserted, err := upsertAll(routes, out)
	if err != nil {
		return err
	}

	return deleteAllIf(existing, out,
		func(r *eskip.Route) bool { return !upserted[r.Id] })
}

func deleteCmd(in, out *medium) error {
	routes, err := loadRoutes(in)
	if err != nil {
		return err
	}

	return deleteAllIf(routes, out,
		func(_ *eskip.Route) bool { return true })
}
