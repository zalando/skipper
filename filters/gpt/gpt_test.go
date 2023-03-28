package gpt

import (
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestWaitRequestFilter_Request(t *testing.T) {
	const timeout = 1 * time.Second
	fr := make(filters.Registry)
	fr.Register(NewWait())

	r := &eskip.Route{
		Filters: []*eskip.Filter{{
			Name: "wait",
			Args: []interface{}{timeout.String()},
		}},
		BackendType: eskip.ShuntBackend,
	}

	pr := proxytest.New(fr, r)
	defer pr.Close()

	path := pr.URL

	start := time.Now()
	_, err := http.Get(path)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if duration < timeout {
		t.Errorf("request took less than %v", timeout)
	}
}
