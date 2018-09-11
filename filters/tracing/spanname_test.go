package tracing

import (
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func Test(t *testing.T) {
	const spanName = "test-span"

	f, err := NewSpanName().CreateFilter([]interface{}{spanName})
	if err != nil {
		t.Fatal(err)
	}

	var ctx filtertest.Context
	ctx.FStateBag = make(map[string]interface{})

	f.Request(&ctx)
	bag := ctx.StateBag()
	if bag[OpenTracingProxySpanKey] != spanName {
		t.Error("failed to set the span name")
	}
}
