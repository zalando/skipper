package settings

import "skipper/skipper"
import "github.com/mailgun/route"

const defaultAddress = ":9090"

type jsonmap map[string]interface{}
type jsonlist []interface{}

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

func createFilter(ref jsonmap, specs map[string]jsonmap, mwr skipper.MiddlewareRegistry) skipper.Filter {
	id, _ := ref["id"].(string)
	spec := specs[id]
	mwn, _ := spec["middleware-name"].(string)
	mw := mwr.Get(mwn)
	if mw == nil {
		return nil
	}

	prio, _ := ref["priority"].(int64)
	mwc := toJsonmap(spec["config"])
	return mw.MakeFilter(id, int(prio), skipper.MiddlewareConfig(mwc))
}

func processFrontends(
	data interface{},
	backends map[string]*backend,
	filterSpecs map[string]jsonmap,
	mwr skipper.MiddlewareRegistry) []*routedef {

	config := toJsonlist(data)
	processed := []*routedef{}

	for _, raw := range config {
		f := toJsonmap(raw)
		if f == nil {
			continue
		}

		rt, _ := f["route"].(string)
		backendId, _ := f["backendId"].(string)

		filterRefs := toJsonlist(f["filters"])
		filters := make([]skipper.Filter, len(filterRefs))
		for _, ref := range filterRefs {
			filter := createFilter(toJsonmap(ref), filterSpecs, mwr)
			if filter != nil {
				filters = append(filters, filter)
			}
		}

		processed = append(processed, &routedef{
			rt,
			backends[backendId],
			filters})
	}

	return processed
}

func processRaw(rd skipper.RawData, mwr skipper.MiddlewareRegistry) skipper.Settings {
	s := &settings{defaultAddress, route.New()}

	data := rd.GetTestData()
	filterSpecs := processFilterSpecs(data["filter-specs"])
	backends := processBackends(data["backends"])
	routes := processFrontends(data["frontends"], backends, filterSpecs, mwr)

	for _, rt := range routes {
		s.routes.AddRoute(rt.route, rt)
	}

	return s
}
