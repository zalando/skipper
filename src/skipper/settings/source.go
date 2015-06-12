package settings

import "skipper/skipper"
import "github.com/mailgun/route"
import "net/http"
import "sort"

const defaultFeedBufferSize = 32
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

type backend struct {
	url string
}

type filter struct {
	id string
}

type routedef struct {
	route   string
	backend *backend
	filters []skipper.Filter
}

type settings struct {
	address string
	routes  route.Router
}

type Source struct {
	dataClient         skipper.DataClient
	middlewareRegistry skipper.MiddlewareRegistry
	current            skipper.Settings
	get                chan skipper.Settings
}

func getBufferSize() int {
	// todo: return defaultFeedBufferSize when not dev env
	return 0
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

type sortFilters struct {
	filters []skipper.Filter
}

func (sf *sortFilters) Len() int           { return len(sf.filters) }
func (sf *sortFilters) Less(i, j int) bool { return sf.filters[i].Priority() < sf.filters[j].Priority() }
func (sf *sortFilters) Swap(i, j int)      { sf.filters[i], sf.filters[j] = sf.filters[j], sf.filters[i] }

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

		sf := &sortFilters{filters}
		sort.Sort(sf)

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

func (b *backend) Url() string {
	return b.url
}

func (r *routedef) Backend() skipper.Backend {
	return r.backend
}

func (r *routedef) Filters() []skipper.Filter {
	return r.filters
}

func (s *settings) Route(r *http.Request) (skipper.Route, error) {
	rt, err := s.routes.Route(r)
	if rt == nil || err != nil {
		return nil, err
	}

	return rt.(skipper.Route), nil
}

func (s *settings) Address() string {
	return defaultAddress
}

func MakeSource(dc skipper.DataClient, mwr skipper.MiddlewareRegistry) *Source {
	s := &Source{
		dataClient:         dc,
		middlewareRegistry: mwr,
		get:                make(chan skipper.Settings, getBufferSize())}
	go s.feed()
	return s
}

func (ss *Source) Get() <-chan skipper.Settings {
	return ss.get
}

func (ss *Source) feed() {

	// initial skipper
	rd := <-ss.dataClient.Get()
	ss.current = processRaw(rd, ss.middlewareRegistry)

	for {
		select {
		case rd = <-ss.dataClient.Get():
			ss.current = processRaw(rd, ss.middlewareRegistry)
		case ss.get <- ss.current:
		}
	}
}
