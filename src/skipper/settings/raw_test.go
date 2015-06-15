package settings

import "skipper/skipper"
import "net/http"
import "testing"

type testRawData struct {
	data map[string]interface{}
}

func (rd *testRawData) Get() map[string]interface{} { return rd.data }

type testFilter struct {
	id   string
	name string
	data int
}

func (tf *testFilter) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
func (tf *testFilter) Id() string                                       { return tf.id }

type testMiddleware struct{ name string }

func (mw *testMiddleware) Name() string { return mw.name }

func (mw *testMiddleware) MakeFilter(id string, config skipper.MiddlewareConfig) skipper.Filter {
	return &testFilter{
		id:   id,
		name: mw.name,
		data: config["free-data"].(int)}
}

type testMiddlewareRegistry struct {
	mw map[string]skipper.Middleware
}

func (mwr *testMiddlewareRegistry) Add(mw ...skipper.Middleware) {}
func (mwr *testMiddlewareRegistry) Get(name string) skipper.Middleware {
	return mwr.mw[name]
}
func (mwr *testMiddlewareRegistry) Remove(string) {}

func makeTestRequest(url string) *http.Request {
	r, _ := http.NewRequest("GET", url, nil)
	return r
}

func TestParseBackendsFrontendsFilters(t *testing.T) {
	rd := &testRawData{
		map[string]interface{}{
			"backends": map[string]interface{}{
				"backend1": "https://www.zalan.do/backend1",
				"backend2": "https://www.zalan.do/backend2"},

			"frontends": []interface{}{
				map[string]interface{}{
					"route":      "Path(`/frontend1`)",
					"backend-id": "backend1",
					"filters": []interface{}{
						"filter1",
						"filter2"}},

				map[string]interface{}{
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

	mwr := &testMiddlewareRegistry{
		map[string]skipper.Middleware{
			"zal-filter-1": &testMiddleware{"zal-filter-1"},
			"zal-filter-2": &testMiddleware{"zal-filter-2"}}}
	s := processRaw(rd, mwr)

	check := func(req *http.Request, url string,
		filterIds []string, filterNames []string, filterData []int) {

		rt, err := s.Route(req)
		if err != nil {
			t.Error(err)
		}

		if rt.Backend().Url() != url {
			t.Error("invalid url")
		}

		filters := rt.Filters()
		for i, f := range filters {
			filter := f.(*testFilter)
			if filter.id != filterIds[i] ||
				filter.name != filterNames[i] ||
				filter.data != filterData[i] {
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
