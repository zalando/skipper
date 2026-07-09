package openpolicyagent_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestPolicyMaxBodySizeTruncated(t *testing.T) {

	for _, tt := range []struct {
		name        string
		data        string
		contentType string
		wantStatus  int
	}{
		{
			name:        "normal sized body passes",
			data:        `{"hello":"world"}`,
			contentType: "application/json",
			wantStatus:  200,
		},
		{
			name:        "over sized body denied",
			data:        strings.Repeat("A", 1+1024*1024), // 1MB+1B > 1MB default max
			contentType: "application/json",
			wantStatus:  403,
		}} {
		t.Run(tt.name, func(t *testing.T) {

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

import rego.v1

default allow := false

allow if {
    input.truncated_body == false
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
			fr.Register(opaauthorizerequest.NewOpaAuthorizeRequestWithBodySpec(opaRegistry))
			fr.Register(builtin.NewSetPath())
			fr.Register(diag.NewLogHeader())

			docFmt := `r1: * -> logHeader("request") -> opaAuthorizeRequestWithBody("%s") -> "%s";`
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

			inst, err := opaRegistry.GetOrStartInstance(bundleName)
			assert.NoError(t, err)
			assert.NotNil(t, inst)
			assert.Equal(t, true, inst.Healthy())
			t.Logf("opaRegistry running")

			buf := bytes.NewBufferString(tt.data)
			req, err := http.NewRequest("POST", ts.URL, buf)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", tt.contentType)

			rsp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("Failed to get response: %v", err)
			}

			t.Logf("[%s] status=%d", tt.name, rsp.StatusCode)

			if rsp.StatusCode != tt.wantStatus {
				t.Fatalf("[%s] status = %d, want %d", tt.name, rsp.StatusCode, tt.wantStatus)
			}
		})
	}

}
