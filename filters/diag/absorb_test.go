package diag

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestAbsorb(t *testing.T) {
	const (
		bodySize   = 1 << 12
		logTimeout = 3 * time.Second
	)

	l := loggingtest.New()
	defer l.Close()
	a := withLogger(l)
	fr := make(filters.Registry)
	fr.Register(a)
	p := proxytest.New(
		fr,
		&eskip.Route{
			Filters:     []*eskip.Filter{{Name: "absorb"}},
			BackendType: eskip.ShuntBackend,
		},
	)
	defer p.Close()

	req, err := http.NewRequest(
		"POST",
		p.URL,
		io.LimitReader(
			rand.New(rand.NewSource(time.Now().UnixNano())),
			bodySize,
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("X-Flow-Id", "foo-bar-baz")
	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("invalid status code received: %d", rsp.StatusCode)
	}

	check := func(prefix string, err error) {
		if err != nil {
			t.Fatalf("%s: %v", prefix, err)
		}
	}

	check("received", l.WaitFor("received request", logTimeout))
	check("flow ID", l.WaitFor("foo-bar-baz", logTimeout))
	check("consumed", l.WaitFor("consumed", logTimeout))
	check("consumed bytes", l.WaitFor(fmt.Sprint(bodySize), logTimeout))
	check("finished", l.WaitFor("request finished", logTimeout))
}
