package tracingtest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func TestMockTracerSpanTracer(t *testing.T) {
	tracer := tracingtest.NewTracer()

	span := tracer.StartSpan("test")
	span.Finish()

	assert.Same(t, tracer, span.Tracer())
}
