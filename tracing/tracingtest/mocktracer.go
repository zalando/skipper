package tracingtest

import (
	"sync/atomic"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
)

type MockTracer struct {
	mockTracer *mocktracer.MockTracer
	spans      atomic.Int32
}

func NewMockTracer() *MockTracer {
	return &MockTracer{mockTracer: &mocktracer.MockTracer{}}
}

func (t *MockTracer) Reset() {
	t.spans.Store(0)
	t.mockTracer.Reset()
}

func (t *MockTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	t.spans.Add(1)
	return t.mockTracer.StartSpan(operationName, opts...)
}

func (t *MockTracer) FinishedSpans() []*mocktracer.MockSpan {
	timeout := time.After(1 * time.Second)
	retry := time.NewTicker(100 * time.Millisecond)
	defer retry.Stop()
	for {
		finished := t.mockTracer.FinishedSpans()
		if len(finished) == int(t.spans.Load()) {
			return finished
		}
		select {
		case <-retry.C:
		case <-timeout:
			return nil
		}
	}
}

func (t *MockTracer) Inject(sm opentracing.SpanContext, format interface{}, carrier interface{}) error {
	return t.mockTracer.Inject(sm, format, carrier)
}

func (t *MockTracer) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	return t.mockTracer.Extract(format, carrier)
}
