package proxy_test

import (
	"github.com/zalando/skipper/predicates"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
)

// test filter used in TestRequestURIClonedOnSplit
type dependentFilter chan string

func (f dependentFilter) Name() string                                       { return "dependentFilter" }
func (f dependentFilter) CreateFilter([]interface{}) (filters.Filter, error) { return f, nil }
func (f dependentFilter) Response(filters.FilterContext)                     {}

func (f dependentFilter) Request(ctx filters.FilterContext) {
	f <- ctx.Request().RequestURI
}

func TestRequestURIClonedOnSplit(t *testing.T) {
	const routes = `
		main: * -> teeLoopback("test") -> <shunt>;
		shadow: Tee("test") -> dependentFilter() -> <shunt>
	`

	r, err := eskip.Parse(routes)
	if err != nil {
		t.Fatal(err)
	}

	df := make(dependentFilter)
	fr := builtin.MakeRegistry()
	fr.Register(df)

	p := proxytest.WithRoutingOptions(fr, routing.Options{Predicates: []predicates.PredicateSpec{tee.New()}}, r...)
	defer p.Close()

	rsp, err := http.Get(p.URL + "/foo")
	if err != nil {
		t.Fatal(err)
	}

	defer rsp.Body.Close()
	io.Copy(ioutil.Discard, rsp.Body)

	select {
	case uri := <-df:
		if uri != "/foo" {
			t.Fatalf("expected /foo, got: %s", uri)
		}
	case <-time.After(120 * time.Millisecond):
		t.Fatal("test timeout")
	}
}
