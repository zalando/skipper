package tracingtest

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
)

type MockTracer struct {
	mockTracer *mocktracer.MockTracer
	spans      atomic.Int32
}

type MockSpan struct {
	*mocktracer.MockSpan
	t *MockTracer
}

var _ opentracing.Tracer = NewTracer()

func NewTracer() *MockTracer {
	return &MockTracer{mockTracer: mocktracer.New()}
}

func (t *MockTracer) Reset() {
	t.spans.Store(0)
	t.mockTracer.Reset()
}

func (t *MockTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	t.spans.Add(1)
	return &MockSpan{MockSpan: t.mockTracer.StartSpan(operationName, opts...).(*mocktracer.MockSpan), t: t}
}

func (t *MockTracer) FinishedSpans() []*MockSpan {
	timeout := time.After(1 * time.Second)
	retry := time.NewTicker(100 * time.Millisecond)
	defer retry.Stop()
	for {
		finished := t.mockTracer.FinishedSpans()
		if len(finished) == int(t.spans.Load()) {
			result := make([]*MockSpan, len(finished))
			for i, s := range finished {
				result[i] = &MockSpan{MockSpan: s, t: t}
			}
			return result
		}
		select {
		case <-retry.C:
		case <-timeout:
			panic(fmt.Sprintf("Timeout waiting for %d finished spans, got: %d", t.spans.Load(), len(finished)))
		}
	}
}

func (t *MockTracer) FindSpan(operationName string) *MockSpan {
	for _, s := range t.FinishedSpans() {
		if s.OperationName == operationName {
			return s
		}
	}
	return nil
}

func (t *MockTracer) Inject(sm opentracing.SpanContext, format any, carrier any) error {
	return t.mockTracer.Inject(sm, format, carrier)
}

func (t *MockTracer) Extract(format any, carrier any) (opentracing.SpanContext, error) {
	return t.mockTracer.Extract(format, carrier)
}

func (s *MockSpan) Tracer() opentracing.Tracer {
	return s.t
}
