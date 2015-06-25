package settings

import (
	"errors"
	"fmt"
	"github.com/mailgun/route"
	"net/url"
	"skipper/skipper"
)

const (
	defaultAddress = ":9090"
	shuntBackendId = "<shunt>"
)

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
	if data == nil {
		return processed
	}

	config := data.(map[string]interface{})
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
			if u, err := url.ParseRequestURI(us); err == nil {
				processed[id] = &backend{u.Scheme, u.Host, false}
			}
		}
	}

	return processed
}

func createFilter(id string, specs map[string]jsonmap, mwr skipper.FilterRegistry) (skipper.Filter, error) {
	spec := specs[id]
	mwn, _ := spec["filter-spec"].(string)
	mw := mwr.Get(mwn)
	if mw == nil {
		return nil, errors.New(fmt.Sprintf("filter not found: '%s' '%s'", id, mwn))
	}

	mwc := toJsonmap(spec["config"])
	return mw.MakeFilter(id, skipper.FilterConfig(mwc))
}

func processFrontends(
	data interface{},
	backends map[string]*backend,
	filterSpecs map[string]jsonmap,
	mwr skipper.FilterRegistry) ([]*routedef, error) {

	config := toJsonmap(data)
	processed := []*routedef{}
	shunt := &backend{"", "", true}

	for _, raw := range config {
		f := toJsonmap(raw)
		if f == nil {
			continue
		}

		rt, _ := f["route"].(string)
		backendId, _ := f["backend-id"].(string)

		var b *backend
		if backendId == shuntBackendId {
			b = shunt
		} else {
			b = backends[backendId]
		}

		filterRefs := toJsonlist(f["filters"])
		filters := []skipper.Filter{}
		for _, id := range filterRefs {
			filter, err := createFilter(id.(string), filterSpecs, mwr)
			if err != nil {
				return nil, err
			}

			filters = append(filters, filter)
		}

		// todo: if no backend, should be an error
		processed = append(processed, &routedef{rt, b, filters})
	}

	return processed, nil
}

func processRaw(rd skipper.RawData, mwr skipper.FilterRegistry) (skipper.Settings, error) {
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
