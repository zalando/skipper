package builtin_test

import (
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
)

func TestHedgeFilterFactory(t *testing.T) {
	spec, ok := builtin.MakeRegistry()["hedge"]
	if !ok {
		t.Fatal("hedge filter is not registered")
	}
	if spec.Name() != "hedge" {
		t.Fatalf("filter name = %q, want hedge", spec.Name())
	}
	if _, err := spec.CreateFilter(nil); err != nil {
		t.Fatalf("zero-argument filter creation failed: %v", err)
	}
	if _, err := spec.CreateFilter([]any{"unexpected"}); err != filters.ErrInvalidFilterParameters {
		t.Fatalf("filter creation error = %v, want %v", err, filters.ErrInvalidFilterParameters)
	}
}
