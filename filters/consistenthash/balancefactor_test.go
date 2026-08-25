package consistenthash

import (
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/loadbalancer"
)

func TestConsistentHashBalanceFactor(t *testing.T) {
	spec := NewConsistentHashBalanceFactor()
	if spec.Name() != "consistentHashBalanceFactor" {
		t.Error("wrong name")
	}

	c := &filtertest.Context{
		FStateBag: make(map[string]any),
	}

	f, _ := spec.CreateFilter([]any{1.0})
	f.Request(c)

	if c.FStateBag[loadbalancer.ConsistentHashBalanceFactor] != 1.0 {
		t.Error("Failed to set balance factor via filter")
	}

	_, err := spec.CreateFilter([]any{0.5})

	if err == nil {
		t.Error("Expected an error since the balancer factor should be >= 1")
	}
}
