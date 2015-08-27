package settings

import (
	"errors"
	"fmt"
	"github.bus.zalan.do/spearheads/pathtree"
	"github.com/mailgun/route"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/skipper"
	"log"
	"net/http"
	"net/url"
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
	tree *pathtree.Tree
}

type pathParam struct {
    treeParam pathtree.Param
}

type mailgunRouter struct {
    mailgun route.Router
}

func (p *pathParam) Key() string { return p.treeParam.Key }
func (p *pathParam) Value() string { return p.treeParam.Value }

func (mr *mailgunRouter) Route(r *http.Request) (skipper.Route, skipper.PathParams, error) {
    rt, err := mr.mailgun.Route(r)
    return rt.(skipper.Route), nil, err
}

func (t *pathTreeRouter) Route(r *http.Request) (skipper.Route, skipper.PathParams, error) {
	v, params, _ := t.tree.Get(r.URL.Path)

    pparams := make(skipper.PathParams, len(params))
    for i, p := range params {
        pparams[i] = &pathParam{p}
    }

	return v.(skipper.Route), pparams, nil
}

func makePathTreeRouter(routes []*eskip.Route, mwr skipper.FilterRegistry) (skipper.Router, error) {
	pathMap := pathtree.PathMap{}

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
		pathMap[path] = &routedef{b, fs}
	}

	tree, err := pathtree.Make(pathMap)

	if err != nil {
		return nil, err
	}

	return &pathTreeRouter{tree}, nil
}

func formatMatchers(ms []*eskip.Matcher) string {
    fms := make([]string, len(ms))
    for i, m := range ms {
        fargs := make([]string, len(m.Args))
        for j, a := range m.Args {
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

		err = router.AddRoute(formatMatchers(r.Matchers), &routedef{b, fs})
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
