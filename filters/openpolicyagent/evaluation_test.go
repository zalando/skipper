package openpolicyagent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/open-policy-agent/opa/v1/plugins/logs"
	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildConfigWithManualDecisionLogs returns an OPA config with decision_logs using
// trigger=manual so the upload loop never fires automatically. Tests call
// Trigger() explicitly to flush, giving full control over upload timing.
func buildConfigWithManualDecisionLogs(t *testing.T, opaURL, dlServiceURL string) []byte {
	t.Helper()
	return fmt.Appendf(nil, `{
		"services": {
			"test": { "url": %q },
			"dl":   { "url": %q }
		},
		"bundles": {
			"test": { "resource": "/bundles/{{ .bundlename }}" }
		},
		"decision_logs": {
			"service": "dl",
			"reporting": {
				"trigger": "manual"
			}
		},
		"plugins": {
			"envoy_ext_authz_grpc": {
				"path": "envoy/authz/allow",
				"dry-run": false,
				"skip-request-body-parse": false
			}
		}
	}`, opaURL, dlServiceURL)
}

// TestDecisionLogDeliveredAfterClientCancel is the regression test for the bug:
//
//	"Unable to log decision to control plane. err=context canceled"
//
// Before the fix (evaluation.go passing the raw request ctx to logDecision):
// plugin.Log receives a canceled context, dropEvent runs pq.Eval(canceledCtx)
// which immediately returns eval_cancel_error, and the event is silently dropped —
// it never enters the buffer and the upload server receives nothing.
//
// After the fix (context.WithoutCancel): logDecision receives a live context,
// the event is buffered, and after an explicit Trigger() flush it reaches the server.
func TestDecisionLogDeliveredAfterClientCancel(t *testing.T) {
	var uploadCount atomic.Int32

	dlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			uploadCount.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer dlServer.Close()

	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/test", map[string]string{
			"main.rego": `
				package envoy.authz
				default allow = false
			`,
		}),
	)

	config := buildConfigWithManualDecisionLogs(t, opaControlPlane.URL(), dlServer.URL)

	registry, err := NewOpenPolicyAgentRegistry(
		WithReuseDuration(1*time.Second),
		WithCleanInterval(1*time.Second),
		WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)),
	)
	require.NoError(t, err)
	// Close registry before stopping the control plane so background bundle
	// fetches don't race against a stopped server.
	t.Cleanup(func() { registry.Close(); opaControlPlane.Stop() })

	inst, err := registry.GetOrStartInstance("test")
	require.NoError(t, err)

	// Simulate client disconnect: cancel the context before calling Eval.
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	inst.Eval(canceledCtx, &authv3.CheckRequest{ //nolint:errcheck
		Attributes: &authv3.AttributeContext{},
	})

	// Wait for the async decision log goroutine to call logDecision before
	// triggering the upload, so the event is in the plugin buffer.
	syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer syncCancel()
	inst.waitDecisionLogDrained(syncCtx)

	// Explicitly flush the decision log buffer via Trigger.
	// With trigger=manual the periodic loop never fires, so this is the only
	// upload path — giving us a deterministic signal: if the event was buffered,
	// the server receives it; if it was dropped (buggy behavior), it doesn't.
	dlPlugin := logs.Lookup(inst.manager)
	require.NotNil(t, dlPlugin, "expected decision_logs plugin to be registered")
	triggerCtx, triggerCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer triggerCancel()
	_ = dlPlugin.Trigger(triggerCtx)

	assert.Equal(t, int32(1), uploadCount.Load(),
		"decision log was not delivered to the control plane after client cancel")
}

// TestDecisionLogDeliveredAfterContextExpiredDuringEval is the regression test for
// sub-pattern B from purchase-service.log (lines 5 and 9): context expires while
// OPA is evaluating (162ms and 243ms client cancellations).
func TestDecisionLogDeliveredAfterContextExpiredDuringEval(t *testing.T) {
	var uploadCount atomic.Int32

	dlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			uploadCount.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer dlServer.Close()

	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/test", map[string]string{
			"main.rego": `
				package envoy.authz
				default allow = false
			`,
		}),
	)

	config := buildConfigWithManualDecisionLogs(t, opaControlPlane.URL(), dlServer.URL)

	registry, err := NewOpenPolicyAgentRegistry(
		WithReuseDuration(1*time.Second),
		WithCleanInterval(1*time.Second),
		WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)),
	)
	require.NoError(t, err)
	// Close registry before stopping the control plane so background bundle
	// fetches don't race against a stopped server.
	t.Cleanup(func() { registry.Close(); opaControlPlane.Stop() })

	inst, err := registry.GetOrStartInstance("test")
	require.NoError(t, err)

	// Expire the context before entering Eval — simulates a client that timed out
	// partway through (sub-pattern B).
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	inst.Eval(ctx, &authv3.CheckRequest{ //nolint:errcheck
		Attributes: &authv3.AttributeContext{},
	})

	// Wait for the async decision log goroutine to call logDecision before triggering.
	syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer syncCancel()
	inst.waitDecisionLogDrained(syncCtx)

	dlPlugin := logs.Lookup(inst.manager)
	require.NotNil(t, dlPlugin, "expected decision_logs plugin to be registered")
	triggerCtx, triggerCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer triggerCancel()
	_ = dlPlugin.Trigger(triggerCtx)

	assert.Equal(t, int32(1), uploadCount.Load(),
		"decision log was not delivered to the control plane after context expiry")
}
