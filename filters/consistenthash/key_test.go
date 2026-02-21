package consistenthash

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/loadbalancer"
)

func TestConsistentHashKey(t *testing.T) {
	spec := NewConsistentHashKey()
	if spec.Name() != "consistentHashKey" {
		t.Error("wrong name")
	}

	c := &filtertest.Context{
		FRequest: &http.Request{
			Header: http.Header{
				"X-Uid": []string{"user1"},
			},
		},
		FStateBag: make(map[string]any),
	}

	// missing placeholder does not set key
	f, err := spec.CreateFilter([]any{"missing: ${request.header.missing}"})
	if err != nil {
		t.Fatal("failed to create filter")
	}
	f.Request(c)

	if _, ok := c.FStateBag[loadbalancer.ConsistentHashKey]; ok {
		t.Error("unexpected key")
	}

	// set key with placeholder
	f, _ = spec.CreateFilter([]any{"set: ${request.header.x-uid}"})
	f.Request(c)

	if c.FStateBag[loadbalancer.ConsistentHashKey] != "set: user1" {
		t.Error("wrong key")
	}

	// second filter overwrites
	f, _ = spec.CreateFilter([]any{"overwrite: ${request.header.x-uid}"})
	f.Request(c)

	if c.FStateBag[loadbalancer.ConsistentHashKey] != "overwrite: user1" {
		t.Error("overwrite expected")
	}

	// missing placeholder does not overwrite
	f, _ = spec.CreateFilter([]any{"missing overwrite: ${request.header.missing}"})
	f.Request(c)

	if c.FStateBag[loadbalancer.ConsistentHashKey] != "overwrite: user1" {
		t.Error("unexpected overwrite")
	}
}
