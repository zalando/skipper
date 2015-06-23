package settings

import (
	"net/http"
	"net/url"
	"skipper/mock"
	"skipper/skipper"
	"testing"
)

func makeTestRequest(url string) *http.Request {
	r, _ := http.NewRequest("GET", url, nil)
	return r
}

func TestParseBackendsFrontendsFilters(t *testing.T) {
	rd := &mock.RawData{
		map[string]interface{}{
			"backends": map[string]interface{}{
				"backend1": "https://www.zalan.do/backend1",
				"backend2": "https://www.zalan.do/backend2"},

			"frontends": map[string]interface{}{
				"frontend1": map[string]interface{}{
					"route":      "Path(`/frontend1`)",
					"backend-id": "backend1",
					"filters": []interface{}{
						"filter1",
						"filter2"}},

				"frontend2": map[string]interface{}{
					"route":      "Path(`/frontend2`)",
					"backend-id": "backend2",
					"filters": []interface{}{
						"filter3",
						"filter4"}}},

			"filter-specs": map[string]interface{}{
				"filter1": map[string]interface{}{
					"middleware-name": "zal-filter-1",
					"config": map[string]interface{}{
						"free-data": 2}},

				"filter2": map[string]interface{}{
					"middleware-name": "zal-filter-2",
					"config": map[string]interface{}{
						"free-data": 4}},

				"filter3": map[string]interface{}{
					"middleware-name": "zal-filter-1",
					"config": map[string]interface{}{
						"free-data": 8}},

				"filter4": map[string]interface{}{
					"middleware-name": "zal-filter-2",
					"config": map[string]interface{}{
						"free-data": 16}}}}}

	mwr := &mock.MiddlewareRegistry{
		map[string]skipper.Middleware{
			"zal-filter-1": &mock.Middleware{"zal-filter-1"},
			"zal-filter-2": &mock.Middleware{"zal-filter-2"}}}
	s, _ := processRaw(rd, mwr)

	check := func(req *http.Request, u string,
		filterIds []string, filterNames []string, filterData []int) {

		rt, err := s.Route(req)
		if err != nil {
			t.Error(err)
		}

		up, _ := url.ParseRequestURI(u)

		if rt.Backend().Scheme() != up.Scheme || rt.Backend().Host() != up.Host {
			t.Error("invalid url")
		}

		filters := rt.Filters()
		for i, f := range filters {
			filter := f.(*mock.Filter)
			if filter.FId != filterIds[i] ||
				filter.Name != filterNames[i] ||
				filter.Data != filterData[i] {
				t.Error("invalid filter settings")
			}
		}
	}

	check(
		makeTestRequest("https://www.zalan.do/frontend1"),
		"https://www.zalan.do/backend1",
		[]string{"filter1", "filter2"},
		[]string{"zal-filter-1", "zal-filter-2"},
		[]int{2, 4})

	check(
		makeTestRequest("https://www.zalan.do/frontend2"),
		"https://www.zalan.do/backend2",
		[]string{"filter3", "filter4"},
		[]string{"zal-filter-1", "zal-filter-2"},
		[]int{8, 16})
}
