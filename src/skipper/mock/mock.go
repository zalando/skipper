package mock

import "skipper/skipper"
import "github.com/mailgun/route"
import "net/http"

type RawData struct {
	Data map[string]interface{}
}

type DataClient struct {
	FReceive chan skipper.RawData
}

type Backend struct {
	FUrl string
}

type Filter struct {
	FId             string
	Name            string
	Data            int
	RequestHeaders  map[string]string
	ResponseHeaders map[string]string
}

type Route struct {
	FBackend *Backend
	FFilters []skipper.Filter
}

type Settings struct {
	RouterImpl route.Router
}

type Middleware struct{ FName string }

type MiddlewareRegistry struct {
	Middleware map[string]skipper.Middleware
}

func (rd *RawData) Get() map[string]interface{} { return rd.Data }

func MakeDataClient(data map[string]interface{}) *DataClient {
	dc := &DataClient{make(chan skipper.RawData)}
	dc.Feed(data)
	return dc
}

func (dc *DataClient) Receive() <-chan skipper.RawData {
	return dc.FReceive
}

func (dc *DataClient) Feed(data map[string]interface{}) {
	go func() {
		dc.FReceive <- &RawData{data}
	}()
}

func (b *Backend) Url() string {
	return b.FUrl
}

func (f *Filter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	copyMap := func(to http.Header, from map[string]string) {
		for k, v := range from {
			to.Set(k, v)
		}
	}

	copyMap(r.Header, f.RequestHeaders)
	copyMap(w.Header(), f.ResponseHeaders)
}

func (f *Filter) Id() string {
	return f.FId
}

func (r *Route) Backend() skipper.Backend {
	return r.FBackend
}

func (r *Route) Filters() []skipper.Filter {
	return r.FFilters
}

func MakeSettings(url string, filters []skipper.Filter) *Settings {
	rt := route.New()
	b := &Backend{url}
	r := &Route{b, filters}
	rt.AddRoute("Path(\"/hello/<v>\")", r)
	return &Settings{rt}
}

func (s *Settings) Route(r *http.Request) (skipper.Route, error) {
	rt, err := s.RouterImpl.Route(r)
	if rt == nil || err != nil {
		return nil, err
	}

	return rt.(skipper.Route), nil
}

func (s *Settings) Address() string {
	return ":9090"
}

func (mw *Middleware) Name() string { return mw.FName }

func (mw *Middleware) MakeFilter(id string, config skipper.MiddlewareConfig) skipper.Filter {
	return &Filter{
		FId:  id,
		Name: mw.FName,
		Data: config["free-data"].(int)}
}

func (mwr *MiddlewareRegistry) Add(mw ...skipper.Middleware) {
	for _, mwi := range mw {
		mwr.Middleware[mwi.Name()] = mwi
	}
}

func (mwr *MiddlewareRegistry) Get(name string) skipper.Middleware {
	return mwr.Middleware[name]
}

func (mwr *MiddlewareRegistry) Remove(name string) {
	delete(mwr.Middleware, name)
}

// where missing
//
// func (rd *testData) Get() map[string]interface{} {
// 	return map[string]interface{}{
// 		"backends": map[string]interface{}{"hello": "http://localhost:9999/slow"},
// 		"frontends": []interface{}{
// 			map[string]interface{}{
// 				"route":      "Path(\"/hello\")",
// 				"backend-id": "hello"}}}
// }
