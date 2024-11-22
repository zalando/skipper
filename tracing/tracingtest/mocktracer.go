package tracingtest

import (
	"sync/atomic"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
)

type MockTracer struct {
	*mocktracer.MockTracer
	spans int32
}

func (t *MockTracer) Reset() {
	atomic.StoreInt32(&t.spans, 0)
	t.MockTracer.Reset()
}

func (t *MockTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	atomic.AddInt32(&t.spans, 1)
	return t.MockTracer.StartSpan(operationName, opts...)
}

func (t *MockTracer) FinishedSpans() []*mocktracer.MockSpan {
	timeout := time.After(1 * time.Second)
	retry := time.NewTicker(100 * time.Millisecond)
	defer retry.Stop()
	for {
		finished := t.MockTracer.FinishedSpans()
		if len(finished) == int(atomic.LoadInt32(&t.spans)) {
			return finished
		}
		select {
		case <-retry.C:
		case <-timeout:
			return nil
		}
	}
}
