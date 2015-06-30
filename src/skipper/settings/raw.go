package settings

import (
	"errors"
	"eskip"
	"fmt"
	"github.com/mailgun/route"
	"log"
	"net/url"
	"skipper/skipper"
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

func processRaw(rd skipper.RawData, mwr skipper.FilterRegistry) (skipper.Settings, error) {
	d, err := eskip.Parse(rd.Get())
	if err != nil {
		return nil, err
	}

	s := &settings{route.New()}
	for _, r := range d {
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

		s.routes.AddRoute(r.MatchExp, &routedef{r.MatchExp, b, fs})
	}

	return s, nil
}
