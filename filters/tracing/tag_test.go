package tracing

import (
	"net/http"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func TestTracingTag(t *testing.T) {
	const tagName = "test_tag"
	const tagValue = "test_value"

	req := &http.Request{}

	span := tracingtest.NewSpan("proxy")
	req = req.WithContext(opentracing.ContextWithSpan(req.Context(), span))
	ctx := &filtertest.Context{FRequest: req}
	s := NewTag()
	f, err := s.CreateFilter([]interface{}{tagName, tagValue})
	if err != nil {
		t.Error(err)
		return
	}

	f.Request(ctx)

	v, ok := span.Tags[tagName]
	if !ok {
		t.Error("tag was not set")
	}

	vs, ok := v.(string)
	if !ok || vs != tagValue {
		t.Error("invalid header value was copied")
	}
}
