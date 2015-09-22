// provides test implementations for the interfaces in the skipper package.
// it can start an etcd instance, too, if the github.com/coreos/etcd package was installed.
package mock

import (
	"github.com/zalando/skipper/skipper"
	"github.com/zalando/skipper/requestmatch"
	"net/http"
	"net/url"
    "errors"
)

type RawData struct {
	Data string
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
	FServed         bool
	FStateBag       skipper.StateBag
}

type Filter struct {
	FId             string
	Name            string
	Data            float64
	RequestHeaders  map[string]string
	ResponseHeaders map[string]string
}

type Route struct {
	FBackend *Backend
	FFilters []skipper.Filter
}

type Settings struct {
	RouterImpl *requestmatch.Matcher
}

type FilterSpec struct{ FName string }

type FilterRegistry struct {
	FilterSpec map[string]skipper.FilterSpec
}

type routeDefinition struct {
    path string
    method string
    route skipper.Route
}

func (rd *RawData) Get() string { return rd.Data }

func MakeDataClient(data string) *DataClient {
	dc := &DataClient{make(chan skipper.RawData)}
	dc.Feed(data)
	return dc
}

func (dc *DataClient) Receive() <-chan skipper.RawData {
	return dc.FReceive
}

func (dc *DataClient) Feed(data string) {
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

func (fc *FilterContext) StateBag() skipper.StateBag {
	return fc.FStateBag
}

func (fc *FilterContext) MarkServed() {
	fc.FServed = true
}

func (fc *FilterContext) IsServed() bool {
	return fc.FServed
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

func (rd *routeDefinition) Path() string { return rd.path }
func (rd *routeDefinition) Method() string { return "" }
func (rd *routeDefinition) HostRegexps() []string { return nil }
func (rd *routeDefinition) PathRegexps() []string { return nil }
func (rd *routeDefinition) Headers() map[string]string { return nil }
func (rd *routeDefinition) HeaderRegexps() map[string][]string { return nil }
func (rd *routeDefinition) Value() interface{} { return rd.route }

func MakeSettings(u string, filters []skipper.Filter, shunt bool) (*Settings, error) {
	up, _ := url.Parse(u)
	rt, errs := requestmatch.Make([]requestmatch.Definition{
        &routeDefinition{
            path: "/hello/*param",
            route: &Route{&Backend{FScheme: up.Scheme, FHost: up.Host, FIsShunt: shunt}, filters}}},
        false)
    if len(errs) != 0 {
        return nil, errors.New("failed to create request matcher")
    }

	return &Settings{rt}, nil
}

func MakeSettingsWithRoutes(routes map[string]skipper.Route) (*Settings, error) {
    var defs []requestmatch.Definition
    for p, r := range routes {
        defs = append(defs, &routeDefinition{path: p, route: r})
    }

    rt, errs := requestmatch.Make(defs, false)
    if len(errs) != 0 {
        return nil, errors.New("failed to create matcher")
    }

    return &Settings{rt}, nil
}

func (s *Settings) Route(r *http.Request) (skipper.Route, error) {
	rt, _ := s.RouterImpl.Match(r)
	if rt == nil {
		return nil, nil
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
		Data: config[0].(float64)}, nil
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
