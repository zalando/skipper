package openpolicyagent_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/filters/openpolicyagent/opaauthorizerequest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestPolicyPathBypass(t *testing.T) {

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("backend")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	bundleName := "test-bundle"

	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/"+bundleName, map[string]string{
			"main.rego": `
package envoy.authz

default allow := false

allow if {
	not startswith(input.attributes.request.http.path, "/admin")
}

allow if {
	startswith(input.attributes.request.http.path, "/admin")
	input.attributes.request.http.headers["x-role"] == "admin"
}
			`,
		}),
	)
	defer opaControlPlane.Stop()

	config := fmt.Appendf(nil, `{
		"services": {
			"test": {
				"url": %q
			}
		},
		"bundles": {
			"test": {
				"resource": "/bundles/{{ .bundlename }}"
			}
		},
		"labels": {
			"environment": "test"
		},
		"plugins": {
			"envoy_ext_authz_grpc": {
				"path": "envoy/authz/allow",
				"dry-run": false
			}
		}
	}`, opaControlPlane.URL())

	opts := []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error{
		openpolicyagent.WithConfigTemplate(config),
	}

	opaRegistry, err := openpolicyagent.NewOpenPolicyAgentRegistry(
		openpolicyagent.WithPreloadingEnabled(true),
		openpolicyagent.WithEnableDataPreProcessingOptimization(true),
		openpolicyagent.WithInstanceStartupTimeout(5*time.Second),
		openpolicyagent.WithMaxRequestBodyBytes(1024*1024),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
	)
	if err != nil {
		t.Fatalf("opaRegistry: %v", err)
	}
	defer opaRegistry.Close()

	fr := make(filters.Registry)
	fr.Register(opaauthorizerequest.NewOpaAuthorizeRequestSpec(opaRegistry))
	fr.Register(builtin.NewStatus())
	fr.Register(builtin.NewInlineContent())
	fr.Register(diag.NewLogHeader())

	docFmt := `r1: * -> status(200) -> inlineContent("OK") -> <shunt>;
r2: Path("/admin") -> logHeader("request") -> opaAuthorizeRequest("%s") -> "%s";`
	r := eskip.MustParse(fmt.Sprintf(docFmt, bundleName, backend.URL))
	dc := testdataclient.New(r)
	defer dc.Close()

	t.Logf("routing starting")
	ro := routing.Options{
		FilterRegistry:  fr,
		DataClients:     []routing.DataClient{dc},
		PreProcessors:   []routing.PreProcessor{opaRegistry.NewPreProcessor()},
		PostProcessors:  []routing.PostProcessor{opaRegistry},
		PollTimeout:     time.Second,
		SuppressLogs:    false,
		SignalFirstLoad: true,
	}
	rt := routing.New(ro)
	defer rt.Close()

	<-rt.FirstLoad()
	t.Logf("routing started")

	pr := proxy.WithParams(proxy.Params{
		Routing: rt,
	})
	defer pr.Close()

	t.Logf("proxy running")

	ts := httptest.NewServer(pr)
	defer ts.Close()

	var inst *openpolicyagent.OpenPolicyAgentInstance

	for range 15 {
		inst, err = opaRegistry.GetOrStartInstance(bundleName)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if !inst.Healthy() {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
	assert.NoError(t, err)
	assert.NotNil(t, inst)
	assert.Equal(t, true, inst.Healthy())
	t.Logf("opaRegistry running")

	for _, tt := range []struct {
		name string
		path string
		role string
		want int
	}{
		{
			name: "test /admin with admin role is allowed",
			path: "/admin",
			role: "admin",
			want: 200,
		},
		{
			name: "test /public without role is allowed",
			path: "/public",
			role: "no role",
			want: 200,
		},
		{
			name: "test /admin without role is denied",
			path: "/admin",
			role: "no role",
			want: 403,
		},
		{
			name: "test //admin without role is denied",
			path: "//admin",
			role: "no role",
			want: 403,
		},
		{
			name: "test /./admin without role is denied",
			path: "/./admin",
			role: "no role",
			want: 403,
		},
		{
			name: "test /x/../admin without role is denied",
			path: "/x/../admin",
			role: "no role",
			want: 403,
		},
		{
			name: "test /x/%2e%2e/admin without role is denied",
			path: "/x/%2e%2e/admin",
			role: "no role",
			want: 403,
		},
		{
			name: "test /%2e/admin without role is denied",
			path: "/%2e/admin",
			role: "no role",
			want: 403,
		},
		{
			name: "test /%2fadmin without role is denied",
			path: "/%2fadmin",
			role: "no role",
			want: 403,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL+tt.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if tt.role != "" {
				req.Header.Set("x-role", tt.role)
			}

			rsp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("Failed to get response: %v", err)
			}

			t.Logf("[%s] status=%d", tt.name, rsp.StatusCode)

			if rsp.StatusCode != tt.want {
				t.Fatalf("[%s] got status = %d, want %d", tt.name, rsp.StatusCode, tt.want)
			}
		})
	}

}
