package settings

import (
	"github.com/zalando/skipper/mock"
	"github.com/zalando/skipper/skipper"
	"net/http"
	"net/url"
	"testing"
)

func makeTestRequest(url string) *http.Request {
	r, _ := http.NewRequest("GET", url, nil)
	return r
}

func TestParseBackendsFrontendsFilters(t *testing.T) {
	rd := &mock.RawData{`

        frontend1:
            Path("/frontend1") ->
            zalFilter1(2) ->
            zalFilter2(4) ->
            "https://www.zalan.do/backend1";

        frontend2:
            Path("/frontend2") ->
            zalFilter1(8) ->
            zalFilter2(16) ->
            "https://www.zalan.do/backend2";

    `}

	mwr := &mock.FilterRegistry{
		map[string]skipper.FilterSpec{
			"zalFilter1": &mock.FilterSpec{"zalFilter1"},
			"zalFilter2": &mock.FilterSpec{"zalFilter2"}}}
	s, _ := processRaw(rd, mwr)

	check := func(req *http.Request, u string, filterNames []string, filterData []float64) {

		rt, err := s.Route(req)
		if err != nil {
			t.Error(err)
		}

		up, _ := url.ParseRequestURI(u)

		if rt == nil {
			t.Error("invalid route")
			return
		}

		if rt.Backend().Scheme() != up.Scheme || rt.Backend().Host() != up.Host {
			t.Error("invalid url")
		}

		filters := rt.Filters()
		for i, f := range filters {
			filter := f.(*mock.Filter)
			if filter.Name != filterNames[i] ||
				filter.Data != filterData[i] {
				t.Error("invalid filter settings")
			}
		}
	}

	check(
		makeTestRequest("https://www.zalan.do/frontend1"),
		"https://www.zalan.do/backend1",
		[]string{"zalFilter1", "zalFilter2"},
		[]float64{2, 4})

	check(
		makeTestRequest("https://www.zalan.do/frontend2"),
		"https://www.zalan.do/backend2",
		[]string{"zalFilter1", "zalFilter2"},
		[]float64{8, 16})
}

func TestCreatesShuntBackend(t *testing.T) {
	rd := &mock.RawData{`frontend1: Path("/frontend1") -> <shunt>`}
	mwr := &mock.FilterRegistry{}
	s, _ := processRaw(rd, mwr)
	r := makeTestRequest("https://www.zalan.do/frontend1")
	rt, err := s.Route(r)
	if err != nil {
		t.Error("failed to create route with shunt")
		return
	}

	if rt == nil {
		t.Error("invalid route")
		return
	}

	if !rt.Backend().IsShunt() {
		t.Error("failed to create route with shunt")
	}
}
