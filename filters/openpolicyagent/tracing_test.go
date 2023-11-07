package openpolicyagent

import (
	"context"
	"net/http"
	"testing"

	opatracing "github.com/open-policy-agent/opa/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/tracing"
	"github.com/zalando/skipper/tracing/tracingtest"
)

type MockTransport struct {
}

func (t *MockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{}, nil
}

func TestTracingFactory(t *testing.T) {
	tracer := &tracingtest.Tracer{}
	tw := &tracing.TracerWrapper{Ot: tracer}
    f := &tracingFactory{tracer: tw}

	tr := f.NewTransport(&MockTransport{}, opatracing.Options{&OpenPolicyAgentInstance{}})

	ctx, span := tw.Start(context.Background(), "open-policy-agent")

	req := &http.Request{
		Header: map[string][]string{},
	}
	req = req.WithContext(ctx)

	_, err := tr.RoundTrip(req)
	assert.NoError(t, err)

	span.End()
	_, ok := tracer.FindSpan("http.send")
	assert.True(t, ok, "No http.send span was created")
}
