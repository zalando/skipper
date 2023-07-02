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

const (
	bodySize   = 1 << 12
	logTimeout = 3 * time.Second
)

func testAbsorb(t *testing.T, silent bool) {
	l := loggingtest.New()
	defer l.Close()
	a := withLogger(silent, l)
	fr := make(filters.Registry)
	fr.Register(a)
	p := proxytest.New(
		fr,
		&eskip.Route{
			Filters:     []*eskip.Filter{{Name: a.Name()}},
			BackendType: eskip.ShuntBackend,
		},
	)
	defer p.Close()

	req, err := http.NewRequest(
		"POST",
		p.URL,
		io.LimitReader(
			rand.New(rand.NewSource(rand.Int63())),
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

	expectLog := func(content string, err error) {
		if err != nil {
			t.Fatalf("%s: %v", content, err)
		}
	}

	expectNoLog := func(content string, err error) {
		if err != loggingtest.ErrWaitTimeout {
			t.Fatalf("%s: unexpected log entry", content)
		}
	}

	for _, content := range []string{
		"received request",
		"foo-bar-baz",
		"consumed",
		fmt.Sprint(bodySize),
		"request finished",
	} {
		err := l.WaitFor(content, logTimeout)
		if silent {
			expectNoLog(content, err)
			continue
		}

		expectLog(content, err)
	}
}

func TestAbsorb(t *testing.T) {
	if NewAbsorb().Name() != filters.AbsorbName {
		t.Error("wrong filter name")
	}
	testAbsorb(t, false)
}

func TestAbsorbSilent(t *testing.T) {
	if NewAbsorbSilent().Name() != filters.AbsorbSilentName {
		t.Error("wrong filter name")
	}
	testAbsorb(t, true)
}
