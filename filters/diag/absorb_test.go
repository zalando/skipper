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

func testAbsorb(t *testing.T, silent bool) {
	const bodySize = 1 << 12

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

	body := io.LimitReader(rand.New(rand.NewSource(time.Now().UnixNano())), bodySize)
	req, err := http.NewRequest("POST", p.URL, body)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("X-Flow-Id", "foo-bar-baz")
	rsp, err := p.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("invalid status code received: %d", rsp.StatusCode)
	}

	const lastMessage = "testAbsorb finished"
	l.Info(lastMessage)
	if err := l.WaitFor(lastMessage, 100*time.Millisecond); err != nil {
		t.Errorf("Expected %s: %v", lastMessage, err)
	}

	if silent {
		if err := l.WaitFor("foo-bar-baz", 100*time.Millisecond); err != loggingtest.ErrWaitTimeout {
			t.Error("Unexpected log entry")
		}
	} else {
		for _, content := range []string{
			"received request to be absorbed: foo-bar-baz",
			fmt.Sprintf("request foo-bar-baz, consumed bytes: %d", bodySize),
			"request finished: foo-bar-baz",
		} {
			if err := l.WaitFor(content, 100*time.Millisecond); err != nil {
				t.Errorf("Expected %s: %v", content, err)
			}
		}
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
