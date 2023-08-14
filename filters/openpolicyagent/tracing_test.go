package openpolicyagent

import (
	"context"
	"net/http"
	"testing"

	opatracing "github.com/open-policy-agent/opa/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/tracing/tracingtest"
)

type MockTransport struct {
}

func (t *MockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{}, nil
}

func TestTracingFactory(t *testing.T) {
	f := &tracingFactory{}

	tr := f.NewTransport(&MockTransport{}, opatracing.Options{&OpenPolicyAgentInstance{}})

	tracer := &tracingtest.Tracer{}
	span := tracer.StartSpan("open-policy-agent")
	ctx := opentracing.ContextWithSpan(context.Background(), span)

	req := &http.Request{
		Header: map[string][]string{},
	}
	req = req.WithContext(ctx)

	_, err := tr.RoundTrip(req)
	assert.NoError(t, err)

	span.Finish()
	_, ok := tracer.FindSpan("http.send")
	assert.True(t, ok, "No http.send span was created")
}
