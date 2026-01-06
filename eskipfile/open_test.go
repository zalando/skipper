package eskipfile

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
)

func TestOpenFails(t *testing.T) {
	_, err := Open("nonexistent.eskip")
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestOpenSucceeds(t *testing.T) {
	f, err := Open("fixtures/test.eskip")
	if err != nil {
		t.Error(err)
		return
	}

	l := loggingtest.New()
	defer l.Close()

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{f},
		Log:            l,
		PollTimeout:    180 * time.Millisecond,
	})
	defer rt.Close()

	if err := l.WaitFor("route settings applied", 120*time.Millisecond); err != nil {
		t.Error(err)
		return
	}

	check := func(id, path string) {
		r, _ := rt.Route(&http.Request{URL: &url.URL{Path: path}})
		if r == nil || r.Id != id {
			t.Error("failed to load file")
			t.FailNow()
			return
		}
	}

	check("foo", "/foo")
	check("bar", "/bar")
}
