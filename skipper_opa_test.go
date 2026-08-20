package skipper

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
)

const opaTestBundleName = "test"

func writeOpaConfigFile(t *testing.T, serviceURL string) string {
	t.Helper()
	config := fmt.Sprintf(`{
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
	"plugins": {
		"envoy_ext_authz_grpc": {
			"path": "envoy/authz/allow",
			"dry-run": false,
			"skip-request-body-parse": false
		}
	}
}`, serviceURL)

	dir := t.TempDir()
	path := filepath.Join(dir, "opa-config.json")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatalf("failed to write OPA config file: %v", err)
	}
	return path
}

// TestOpaStartup verifies that the full skipper proxy starts up correctly
// with OPA enabled, for both the custom control loop and default loop modes.
// The custom control loop mode exercises the startup fix: startAndTriggerOpaPlugins()
// is now called synchronously at registry creation time.
func TestOpaStartup(t *testing.T) {
	tests := []struct {
		name                    string
		enableCustomControlLoop bool
	}{
		{
			name:                    "custom_control_loop",
			enableCustomControlLoop: true,
		},
		{
			name:                    "default_control_loop",
			enableCustomControlLoop: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opaControlPlane := opasdktest.MustNewServer(
				opasdktest.MockBundle("/bundles/"+opaTestBundleName, map[string]string{
					"main.rego": `
						package envoy.authz
						import rego.v1
						default allow := false
					`,
				}),
			)
			defer opaControlPlane.Stop()

			configFile := writeOpaConfigFile(t, opaControlPlane.URL())

			MuFindAddress.Lock()
			addr := FindAddress(t)
			MuFindAddress.Unlock()

			o := Options{
				Address:                                addr,
				WaitFirstRouteLoad:                     true,
				EnableOpenPolicyAgent:                  true,
				OpenPolicyAgentConfigTemplate:          configFile,
				EnableOpenPolicyAgentCustomControlLoop: tc.enableCustomControlLoop,
				OpenPolicyAgentStartupTimeout:          10 * time.Second,
				OpenPolicyAgentCleanerInterval:         10 * time.Second,
				OpenPolicyAgentControlLoopInterval:     60 * time.Second,
				InlineRoutes: fmt.Sprintf(
					`healthz: Path("/healthz") -> status(200) -> inlineContent("OK") -> <shunt>;`+
						` protected: Path("/protected") -> opaAuthorizeRequest(%q) -> <shunt>;`,
					opaTestBundleName,
				),
			}

			sigs := make(chan os.Signal, 1)
			go RunWithShutdown(o, sigs, nil)
			defer func() { sigs <- syscall.SIGTERM }()

			baseURL := "http://" + addr

			// Wait for proxy readiness.
			var ready bool
			for range 60 {
				rsp, err := http.DefaultClient.Get(baseURL + "/healthz")
				if err == nil && rsp.StatusCode == 200 {
					rsp.Body.Close()
					ready = true
					break
				}
				if rsp != nil {
					rsp.Body.Close()
				}
				time.Sleep(100 * time.Millisecond)
			}
			if !ready {
				t.Fatal("proxy did not become ready in time")
			}

			// OPA policy denies all requests (default allow = false), so we expect 403.
			rsp, err := http.DefaultClient.Get(baseURL + "/protected")
			if err != nil {
				t.Fatalf("request to protected route failed: %v", err)
			}
			defer rsp.Body.Close()

			if rsp.StatusCode != http.StatusForbidden {
				t.Errorf("expected 403 from OPA deny, got %d", rsp.StatusCode)
			}
		})
	}
}

// TestOpaStartupNotReady verifies that when OPA cannot reach its control plane,
// requests to OPA-protected routes are rejected (not silently allowed).
// This exercises the not-ready path that the startup fix prevents from occurring
// on the happy path.
func TestOpaStartupNotReady(t *testing.T) {
	// Point OPA at a non-existent service to trigger startup failure.
	configFile := writeOpaConfigFile(t, "http://127.0.0.1:1")

	MuFindAddress.Lock()
	addr := FindAddress(t)
	MuFindAddress.Unlock()

	o := Options{
		Address:               addr,
		WaitFirstRouteLoad:    true,
		EnableOpenPolicyAgent: true,
		// Do not use custom control loop: instance starts synchronously on first request.
		EnableOpenPolicyAgentCustomControlLoop: false,
		OpenPolicyAgentConfigTemplate:          configFile,
		OpenPolicyAgentStartupTimeout:          500 * time.Millisecond,
		OpenPolicyAgentCleanerInterval:         10 * time.Second,
		InlineRoutes: fmt.Sprintf(
			`healthz: Path("/healthz") -> status(200) -> inlineContent("OK") -> <shunt>;`+
				` protected: Path("/protected") -> opaAuthorizeRequest(%q) -> <shunt>;`,
			opaTestBundleName,
		),
	}

	sigs := make(chan os.Signal, 1)
	go RunWithShutdown(o, sigs, nil)
	defer func() { sigs <- syscall.SIGTERM }()

	baseURL := "http://" + addr

	// Wait for proxy to start (healthz route has no OPA dependency).
	var ready bool
	for range 60 {
		rsp, err := http.DefaultClient.Get(baseURL + "/healthz")
		if err == nil && rsp.StatusCode == 200 {
			rsp.Body.Close()
			ready = true
			break
		}
		if rsp != nil {
			rsp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("proxy did not become ready in time")
	}

	// A request to the OPA-protected route must not be authorized: either the route
	// fails to load (404) because OPA could not start, or OPA denies the request (403/5xx).
	// The proxy must never return 200 (silently allow) when OPA is not ready.
	rsp, err := http.DefaultClient.Get(baseURL + "/protected")
	if err != nil {
		// Connection error is also acceptable evidence that the request was not served.
		return
	}
	defer rsp.Body.Close()

	if rsp.StatusCode == http.StatusOK {
		t.Errorf("expected non-200 when OPA control plane is unreachable, got %d", rsp.StatusCode)
	}
}
