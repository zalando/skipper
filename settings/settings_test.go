package settings

import (
	"github.com/zalando/skipper/filters"
	"net/http"
	"net/url"
	"testing"
)

type mockFilter struct {
	name string
	data float64
}

func (s *mockFilter) CreateFilter(config []interface{}) (filters.Filter, error) {
	return &mockFilter{name: s.name, data: config[0].(float64)}, nil
}

func (s *mockFilter) Name() string                       { return s.name }
func (s *mockFilter) Request(ctx filters.FilterContext)  {}
func (s *mockFilter) Response(ctx filters.FilterContext) {}

func makeTestRequest(url string) *http.Request {
	r, _ := http.NewRequest("GET", url, nil)
	return r
}

func TestParseBackendsFrontendsFilters(t *testing.T) {
	rd := `
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
    `

	fr := filters.Registry{
		"zalFilter1": &mockFilter{name: "zalFilter1"},
		"zalFilter2": &mockFilter{name: "zalFilter2"}}
	s, _ := processRaw(rd, fr, false)

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

		if rt.Backend.Scheme != up.Scheme || rt.Backend.Host != up.Host {
			t.Error("invalid url")
		}

		filters := rt.Filters
		for i, f := range filters {
			filter := f.(*mockFilter)
			if filter.name != filterNames[i] ||
				filter.data != filterData[i] {
				t.Error("invalid filter settings", filter.name, filterNames[i], filter.data, filterData[i])
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
	rd := `frontend1: Path("/frontend1") -> <shunt>`
	fr := make(filters.Registry)
	s, _ := processRaw(rd, fr, false)
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

	if !rt.Backend.IsShunt {
		t.Error("failed to create route with shunt")
	}
}
