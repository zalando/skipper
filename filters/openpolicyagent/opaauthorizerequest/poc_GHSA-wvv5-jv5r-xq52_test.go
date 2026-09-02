package opaauthorizerequest

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/tracing/tracingtest"
)

// TestOPAHeaderInjectionBypassMultiValue verifies GHSA-wvv5-jv5r-xq52 Finding 1:
// a client cannot pre-set headers that the OPA policy injects, and all policy
// values (including multi-value headers) reach the upstream unmodified.
func TestOPAHeaderInjectionBypassMultiValue(t *testing.T) {
	const (
		singleHeader  = "X-Consumer"
		singleValue   = "legitimate-user"
		multiHeader   = "X-Roles"
		multiValue1   = "reader"
		multiValue2   = "writer"
		attackerValue = "attacker-supplied-admin"
	)

	// --- upstream: capture headers as received ---

	var upstreamHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// --- OPA policy: set one single-value and one multi-value header ---

	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/test", map[string]string{
			"main.rego": fmt.Sprintf(`
package envoy.authz

import rego.v1

default allow := {
	"allowed": true,
	"headers": {
		%q: %q,
		%q: [%q, %q]
	}
}
`, singleHeader, singleValue, multiHeader, multiValue1, multiValue2),
		}),
	)
	defer opaControlPlane.Stop()

	config := fmt.Appendf(nil, `{
		"services": {"test": {"url": %q}},
		"bundles": {"test": {"resource": "/bundles/{{ .bundlename }}"}},
		"labels": {"environment": "test"},
		"plugins": {"envoy_ext_authz_grpc": {"path": "envoy/authz/allow", "dry-run": false}}
	}`, opaControlPlane.URL())

	opts := []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error{
		openpolicyagent.WithConfigTemplate(config),
	}
	opaFactory, err := openpolicyagent.NewOpenPolicyAgentRegistry(
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
	)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	fr := make(filters.Registry)
	fr.Register(NewOpaAuthorizeRequestSpec(opaFactory))
	fr.Register(builtin.NewSetPath())

	r := eskip.MustParse(fmt.Sprintf(`* -> opaAuthorizeRequest("test") -> "%s"`, upstream.URL))
	proxy := proxytest.New(fr, r...)
	defer proxy.Close()

	// --- request: attacker pre-sets both policy-controlled headers ---

	req, err := http.NewRequest("GET", proxy.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(singleHeader, attackerValue)
	req.Header.Set(multiHeader, attackerValue)

	resp, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// --- assertions ---

	// Single-value header: must be exactly the policy value.
	if got := upstreamHeaders.Get(singleHeader); got != singleValue {
		t.Errorf("%s: upstream got %q, want %q", singleHeader, got, singleValue)
	}

	// Multi-value header: must contain exactly the two policy values and nothing else.
	gotMulti := upstreamHeaders[http.CanonicalHeaderKey(multiHeader)]
	if len(gotMulti) != 2 {
		t.Fatalf("%s: upstream got %d values %v, want exactly 2", multiHeader, len(gotMulti), gotMulti)
	}
	for _, v := range gotMulti {
		if v == attackerValue {
			t.Errorf("GHSA-wvv5-jv5r-xq52: upstream received attacker-supplied %q in %s", attackerValue, multiHeader)
		}
	}
	if gotMulti[0] != multiValue1 || gotMulti[1] != multiValue2 {
		t.Errorf("%s: upstream got %v, want [%q, %q]", multiHeader, gotMulti, multiValue1, multiValue2)
	}
}
