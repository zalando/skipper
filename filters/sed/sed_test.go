package sed_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/sed"
	"github.com/zalando/skipper/proxy/proxytest"
)

const text = `foobar
baz`

func TestSed(t *testing.T) {
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(text))
	}))

	fr := builtin.MakeRegistry()
	p := proxytest.New(fr, &eskip.Route{
		Filters: []*eskip.Filter{{Name: sed.Name, Args: []interface{}{
			"[a-z][ac-z]*",
			"hoo",
		}}},
		Backend: b.URL,
	})

	rsp, err := http.Get(p.URL)
	if err != nil {
		t.Fatal(err)
	}

	defer rsp.Body.Close()
	d, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		t.Fatal(err)
	}

	const expect = "hoohoo\nhoo"
	if string(d) != expect {
		t.Error("failed to edit stream")
		t.Log("expected:", expect)
		t.Log("got:", string(d))
	}
}
