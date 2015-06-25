// provides test implementations for the interfaces in the skipper package.
// it can start an etcd instance, too, if the github.com/coreos/etcd package was installed.
package mock

import (
	"github.com/mailgun/route"
	"net/http"
	"net/url"
	"skipper/skipper"
)

type RawData struct {
	Data map[string]interface{}
}

type DataClient struct {
	FReceive chan skipper.RawData
}

type Backend struct {
	FScheme  string
	FHost    string
	FIsShunt bool
}

type FilterContext struct {
	FResponseWriter http.ResponseWriter
	FRequest        *http.Request
	FResponse       *http.Response
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

type FilterSpec struct{ FName string }

type FilterRegistry struct {
	FilterSpec map[string]skipper.FilterSpec
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

func (b *Backend) Scheme() string {
	return b.FScheme
}

func (b *Backend) Host() string {
	return b.FHost
}

func (b *Backend) IsShunt() bool {
	return b.FIsShunt
}

func copyHeader(to http.Header, from map[string]string) {
	for k, v := range from {
		to.Set(k, v)
	}
}

func (fc *FilterContext) ResponseWriter() http.ResponseWriter {
	return fc.FResponseWriter
}

func (fc *FilterContext) Request() *http.Request {
	return fc.FRequest
}

func (fc *FilterContext) Response() *http.Response {
	return fc.FResponse
}

func (f *Filter) Request(ctx skipper.FilterContext) {
	copyHeader(ctx.Request().Header, f.RequestHeaders)
}

func (f *Filter) Response(ctx skipper.FilterContext) {
	copyHeader(ctx.Response().Header, f.ResponseHeaders)
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

func MakeSettings(u string, filters []skipper.Filter, shunt bool) *Settings {
	up, _ := url.Parse(u)
	rt := route.New()
	b := &Backend{up.Scheme, up.Host, shunt}
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

func (mw *FilterSpec) Name() string { return mw.FName }

func (mw *FilterSpec) MakeFilter(id string, config skipper.FilterConfig) (skipper.Filter, error) {
	return &Filter{
		FId:  id,
		Name: mw.FName,
		Data: config["free-data"].(int)}, nil
}

func (mwr *FilterRegistry) Add(mw ...skipper.FilterSpec) {
	for _, mwi := range mw {
		mwr.FilterSpec[mwi.Name()] = mwi
	}
}

func (mwr *FilterRegistry) Get(name string) skipper.FilterSpec {
	return mwr.FilterSpec[name]
}

func (mwr *FilterRegistry) Remove(name string) {
	delete(mwr.FilterSpec, name)
}
