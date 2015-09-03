package settings

import (
	"errors"
	"fmt"
	"github.bus.zalan.do/spearheads/pathmux"
	"github.com/mailgun/route"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/skipper"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const shuntBackendId = "<shunt>"

type routeDefinition struct {
	eskipRoute     *eskip.Route
	filterRegistry skipper.FilterRegistry
}

func createBackend(r *eskip.Route) (*backend, error) {
	if r.Shunt {
		return &backend{isShunt: true}, nil
	}

	bu, err := url.ParseRequestURI(r.Backend)
	if err != nil {
		return nil, err
	}

	return &backend{scheme: bu.Scheme, host: bu.Host}, nil
}

func makeFilterId(routeId, filterName string, index int) string {
	return fmt.Sprintf("%s-%s-%d", routeId, filterName, index)
}

func createFilter(id string, spec *eskip.Filter, fr skipper.FilterRegistry) (skipper.Filter, error) {
	mw := fr.Get(spec.Name)
	if mw == nil {
		return nil, errors.New(fmt.Sprintf("filter not found: '%s' '%s'", id, spec.Name))
	}

	return mw.MakeFilter(id, skipper.FilterConfig(spec.Args))
}

func createFilters(r *eskip.Route, fr skipper.FilterRegistry) ([]skipper.Filter, error) {
	fs := make([]skipper.Filter, len(r.Filters))
	for i, fspec := range r.Filters {
		f, err := createFilter(makeFilterId(r.Id, fspec.Name, i), fspec, fr)
		if err != nil {
			return nil, err
		}

		fs[i] = f
	}

	return fs, nil
}

type pathTreeRouter struct {
	tree *pathmux.Tree
}

type mailgunRouter struct {
	mailgun route.Router
}

func (mr *mailgunRouter) Route(r *http.Request) (skipper.Route, skipper.PathParams, error) {
	v, err := mr.mailgun.Route(r)
	rt, _ := v.(skipper.Route)
	return rt, nil, err
}

func (t *pathTreeRouter) Route(r *http.Request) (skipper.Route, skipper.PathParams, error) {
	v, params := t.tree.Lookup(r.URL.Path)
	return v.(skipper.Route), params, nil
}

func makePathTreeRouter(routes []*eskip.Route, fr skipper.FilterRegistry, ignoreTrailingSlash bool) skipper.Router {
	routeDefinitions := make([]RouteDefinition, len(routes))
	for i, rd := range routes {
		routeDefinitions[i] = &routeDefinition{rd, fr}
	}

	router, errs := makeMatcher(routeDefinitions, ignoreTrailingSlash)
	for _, err := range errs {
		log.Println(err)
	}

	return router
}

const wildcardRegexpString = "/((:|\\*)[^/]+)"

var wildcardRegexp *regexp.Regexp

func init() {
	wildcardRegexp = regexp.MustCompile(wildcardRegexpString)
}

func replaceWildCards(p interface{}) interface{} {
	ps, ok := p.(string)
	if !ok {
		return p
	}

	m := wildcardRegexp.FindAllSubmatchIndex([]byte(ps), -1)
	if len(m) == 0 {
		return p
	}

	var parts []string
	var pos int
	for i := 0; i < len(m); i++ {
		if i == 0 {
			pos = 0
		} else {
			pos = m[i-1][3]
		}

		parts = append(parts, ps[pos:m[i][2]], "<", ps[m[i][2]+1:m[i][3]], ">")
	}

	return strings.Join(parts, "")
}

func formatMailgunMatchers(ms []*eskip.Matcher) string {
	fms := make([]string, len(ms))
	for i, m := range ms {
		if m.Name == "Any" {
			fms[i] = "Path(\"/<string>\")"
			continue
		}

		fargs := make([]string, len(m.Args))
		for j, a := range m.Args {
			a = replaceWildCards(a)
			fargs[j] = fmt.Sprintf("`%v`", a)
		}

		fms[i] = fmt.Sprintf("%s(%s)", m.Name, strings.Join(fargs, ", "))
	}

	return strings.Join(fms, " && ")
}

func makeMailgunRouter(routes []*eskip.Route, fr skipper.FilterRegistry) (skipper.Router, error) {
	router := route.New()
	for _, r := range routes {
		b, err := createBackend(r)
		if err != nil {
			log.Println("invalid backend address", r.Id, b, err)
			continue
		}

		fs, err := createFilters(r, fr)
		if err != nil {
			log.Println("invalid filter specification", r.Id, err)
			continue
		}

		err = router.AddRoute(formatMailgunMatchers(r.Matchers), &routedef{b, fs})
		if err != nil {
			log.Println("failed to add route", r.Id, err)
		}
	}
	return &mailgunRouter{router}, nil
}

func processRaw(rd skipper.RawData, fr skipper.FilterRegistry, ignoreTrailingSlash bool) (skipper.Settings, error) {
	d, err := eskip.Parse(rd.Get())
	if err != nil {
		return nil, err
	}

	// TODO: this is the point to switch router implementations
	//	router, err := makeMailgunRouter(d, fr)
	router := makePathTreeRouter(d, fr, ignoreTrailingSlash)
	s := &settings{router}
	return s, nil
}

func (rd *routeDefinition) Id() string {
	return rd.eskipRoute.Id
}

func (rd *routeDefinition) Path() string {
	for _, m := range rd.eskipRoute.Matchers {
		if (m.Name == "Path") && len(m.Args) > 0 {
			p, _ := m.Args[0].(string)
			return p
		}
	}

	return ""
}

func (rd *routeDefinition) HostRegexps() []string {
	var hostRxs []string
	for _, m := range rd.eskipRoute.Matchers {
		if m.Name == "Host" && len(m.Args) > 0 {
			rx, _ := m.Args[0].(string)
			hostRxs = append(hostRxs, rx)
		}
	}

	return hostRxs
}

func (rd *routeDefinition) PathRegexps() []string {
	var pathRxs []string
	for _, m := range rd.eskipRoute.Matchers {
		if m.Name == "PathRegexp" && len(m.Args) > 0 {
			rx, _ := m.Args[0].(string)
			pathRxs = append(pathRxs, rx)
		}
	}

	return pathRxs
}

func (rd *routeDefinition) Method() string {
	for _, m := range rd.eskipRoute.Matchers {
		if m.Name == "Method" && len(m.Args) > 0 {
			method, _ := m.Args[0].(string)
			return method
		}
	}

	return ""
}

func (rd *routeDefinition) Headers() map[string]string {
	headers := make(map[string]string)
	for _, m := range rd.eskipRoute.Matchers {
		if m.Name == "Header" && len(m.Args) >= 2 {
			k, _ := m.Args[0].(string)
			v, _ := m.Args[1].(string)
			headers[k] = v
		}
	}

	return headers
}

func (rd *routeDefinition) HeaderRegexps() map[string][]string {
	headers := make(map[string][]string)
	for _, m := range rd.eskipRoute.Matchers {
		if m.Name == "HeaderRegexp" && len(m.Args) >= 2 {
			k, _ := m.Args[0].(string)
			v, _ := m.Args[1].(string)
			headers[k] = append(headers[k], v)
		}
	}

	return headers
}

func (rd *routeDefinition) Filters() []skipper.Filter {
	fs, err := createFilters(rd.eskipRoute, rd.filterRegistry)
	if err != nil {
		log.Println(err)
	}

	return fs
}

func (rd *routeDefinition) Backend() skipper.Backend {
	b, err := createBackend(rd.eskipRoute)
	if err != nil {
		log.Println(err)
	}

	return b
}
