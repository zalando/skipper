package settings

import (
	"errors"
	"fmt"
	"github.com/dimfeld/httptreemux"
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

func createFilter(id string, spec *eskip.Filter, mwr skipper.FilterRegistry) (skipper.Filter, error) {
	mw := mwr.Get(spec.Name)
	if mw == nil {
		return nil, errors.New(fmt.Sprintf("filter not found: '%s' '%s'", id, spec.Name))
	}

	return mw.MakeFilter(id, skipper.FilterConfig(spec.Args))
}

func createFilters(r *eskip.Route, mwr skipper.FilterRegistry) ([]skipper.Filter, error) {
	fs := make([]skipper.Filter, len(r.Filters))
	for i, fspec := range r.Filters {
		f, err := createFilter(makeFilterId(r.Id, fspec.Name, i), fspec, mwr)
		if err != nil {
			return nil, err
		}

		fs[i] = f
	}

	return fs, nil
}

type pathTreeRouter struct {
	tree *httptreemux.Tree
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
	v, params := t.tree.Search(r.URL.Path)
	return v.(skipper.Route), params, nil
}

func makePathTreeRouter(routes []*eskip.Route, mwr skipper.FilterRegistry) (skipper.Router, error) {
	tree := &httptreemux.Tree{}

	for _, r := range routes {
		// TODO: there is not always a path there
		path := r.Matchers[0].Args[0].(string)
		b, err := createBackend(r)
		if err != nil {
			log.Println("invalid backend address", r.Id, b, err)
			continue
		}
		fs, err := createFilters(r, mwr)
		if err != nil {
			log.Println("invalid filter specification", r.Id, err)
			continue
		}
		err = tree.Add(path, &routedef{b, fs})
		if err != nil {
			log.Println("invalid route path", r.Id, err)
			continue
		}
	}

	return &pathTreeRouter{tree}, nil
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
		fargs := make([]string, len(m.Args))
		for j, a := range m.Args {
			a = replaceWildCards(a)
			fargs[j] = fmt.Sprintf("`%v`", a)
		}

		fms[i] = fmt.Sprintf("%s(%s)", m.Name, strings.Join(fargs, ", "))
	}

	return strings.Join(fms, " && ")
}

func makeMailgunRouter(routes []*eskip.Route, mwr skipper.FilterRegistry) (skipper.Router, error) {
	router := route.New()
	for _, r := range routes {
		b, err := createBackend(r)
		if err != nil {
			log.Println("invalid backend address", r.Id, b, err)
			continue
		}

		fs, err := createFilters(r, mwr)
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

func processRaw(rd skipper.RawData, mwr skipper.FilterRegistry) (skipper.Settings, error) {
	d, err := eskip.Parse(rd.Get())
	if err != nil {
		return nil, err
	}

	// TODO: this is the point to switch router implementations
	//	router, err := makeMailgunRouter(d, mwr)
	router, err := makePathTreeRouter(d, mwr)
	if err != nil {
		return nil, err
	}

	s := &settings{router}

	return s, nil
}
