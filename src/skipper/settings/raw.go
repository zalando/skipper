package settings

import (
	"errors"
	"fmt"
	"github.com/mailgun/route"
	"skipper/skipper"
)

const defaultAddress = ":9090"

type (
	jsonmap  map[string]interface{}
	jsonlist []interface{}
)

func toJsonmap(i interface{}) jsonmap {
	if m, ok := i.(map[string]interface{}); ok {
		return jsonmap(m)
	}

	return nil
}

func toJsonlist(i interface{}) jsonlist {
	if l, ok := i.([]interface{}); ok {
		return jsonlist(l)
	}

	return nil
}

func processFilterSpecs(data interface{}) map[string]jsonmap {
	processed := make(map[string]jsonmap)

	config := toJsonmap(data)
	for id, raw := range config {
		spec := toJsonmap(raw)
		processed[id] = spec
	}

	return processed
}

func processBackends(data interface{}) map[string]*backend {
	processed := make(map[string]*backend)

	config := toJsonmap(data)
	for id, uraw := range config {
		if us, ok := uraw.(string); ok {
			processed[id] = &backend{us}
		}
	}

	return processed
}

func createFilter(id string, specs map[string]jsonmap, mwr skipper.MiddlewareRegistry) (skipper.Filter, error) {
	spec := specs[id]
	mwn, _ := spec["middleware-name"].(string)
	mw := mwr.Get(mwn)
	if mw == nil {
		return nil, errors.New(fmt.Sprintf("middleware not found: %s", mwn))
	}

	mwc := toJsonmap(spec["config"])
	return mw.MakeFilter(id, skipper.MiddlewareConfig(mwc))
}

func processFrontends(
	data interface{},
	backends map[string]*backend,
	filterSpecs map[string]jsonmap,
	mwr skipper.MiddlewareRegistry) ([]*routedef, error) {

	config := toJsonlist(data)
	processed := []*routedef{}

	for _, raw := range config {
		f := toJsonmap(raw)
		if f == nil {
			continue
		}

		rt, _ := f["route"].(string)
		backendId, _ := f["backend-id"].(string)

		filterRefs := toJsonlist(f["filters"])
		filters := []skipper.Filter{}
		for _, id := range filterRefs {
			filter, err := createFilter(id.(string), filterSpecs, mwr)
			if err != nil {
				return nil, err
			}

			filters = append(filters, filter)
		}

		processed = append(processed, &routedef{
			rt,
			backends[backendId],
			filters})
	}

	return processed, nil
}

func processRaw(rd skipper.RawData, mwr skipper.MiddlewareRegistry) (skipper.Settings, error) {
	s := &settings{defaultAddress, route.New()}

	data := rd.Get()
	filterSpecs := processFilterSpecs(data["filter-specs"])
	backends := processBackends(data["backends"])
	routes, err := processFrontends(data["frontends"], backends, filterSpecs, mwr)
	if err != nil {
		return nil, err
	}

	for _, rt := range routes {
		s.routes.AddRoute(rt.route, rt)
	}

	return s, nil
}
