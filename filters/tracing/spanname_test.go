package tracing

import (
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func Test(t *testing.T) {
	const spanName = "test-span"

	f, err := NewSpanName().CreateFilter([]any{spanName})
	if err != nil {
		t.Fatal(err)
	}

	var ctx filtertest.Context
	ctx.FStateBag = make(map[string]any)

	f.Request(&ctx)
	bag := ctx.StateBag()
	if bag[OpenTracingProxySpanKey] != spanName {
		t.Error("failed to set the span name")
	}

	f.Response(&ctx)
}

func TestInvalid(t *testing.T) {
	const spanName = "test-span"

	_, err := NewSpanName().CreateFilter([]any{spanName, "foo"})
	if err == nil {
		t.Fatal(err)
	}

	_, err = NewSpanName().CreateFilter([]any{3})
	if err == nil {
		t.Fatal(err)
	}
}
func TestBoring(t *testing.T) {
	s := NewSpanName().Name()
	if s != filters.TracingSpanNameName {
		t.Fatalf("Wrong name")
	}
}
