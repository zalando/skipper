package opatestutils

import (
	"encoding/json"
	"fmt"
	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

// CreateBundleServers Helper function to create bundle servers
func CreateBundleServers(bundles []string) []*opasdktest.Server {
	var servers []*opasdktest.Server

	for i, bundle := range bundles {
		packageName := fmt.Sprintf("test%d", i+1)
		server := opasdktest.MustNewServer(
			opasdktest.MockBundle(fmt.Sprintf("/bundles/%s", bundle), map[string]string{
				"main.rego": fmt.Sprintf(`
					package %s
					import rego.v1
					default allow := false
				`, packageName),
				".manifest": fmt.Sprintf(`{
					"roots": ["%s"]
				}`, packageName),
			}),
		)
		servers = append(servers, server)
	}

	return servers
}

// CreateMultiBundleConfig Helper function to create multi-bundle configuration
func CreateMultiBundleConfig(servers []*opasdktest.Server) []byte {
	services := make(map[string]interface{})
	bundleConfigs := make(map[string]interface{})

	for i, server := range servers {
		bundleName := fmt.Sprintf("bundle%d", i+1)
		services[bundleName] = map[string]string{"url": server.URL()}
		bundleConfigs[bundleName] = map[string]string{
			"resource": fmt.Sprintf("/bundles/%s", bundleName),
			"service":  bundleName,
		}
	}

	config := map[string]interface{}{
		"services": services,
		"bundles":  bundleConfigs,
		"plugins": map[string]interface{}{
			"envoy_ext_authz_grpc": map[string]interface{}{
				"path":                    "envoy/authz/allow",
				"dry-run":                 false,
				"skip-request-body-parse": false,
			},
		},
	}

	configBytes, _ := json.Marshal(config)
	return configBytes
}

// ControllableBundleServer Wrapper server with controllable availability:
type ControllableBundleServer struct {
	realServer  *opasdktest.Server
	proxyServer *httptest.Server
	available   atomic.Bool
	bundleName  string
}

func StartControllableBundleServer(bundleName string) *ControllableBundleServer {
	realSrv := CreateBundleServers([]string{bundleName})[0]
	cbs := &ControllableBundleServer{
		realServer: realSrv,
		bundleName: bundleName,
	}
	cbs.available.Store(false) // initially unavailable

	cbs.proxyServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cbs.available.Load() {
			w.WriteHeader(http.StatusTooManyRequests) // 429
			w.Write([]byte("Bundle temporarily unavailable"))
			return
		}

		// Proxy request to real bundle server
		proxyURL := cbs.realServer.URL() + r.URL.Path
		resp, err := http.Get(proxyURL)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failed to fetch bundle"))
			return
		}
		defer resp.Body.Close()

		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))

	return cbs
}

func (c *ControllableBundleServer) SetAvailable(yes bool) {
	c.available.Store(yes)
}

func (c *ControllableBundleServer) URL() string {
	return c.proxyServer.URL
}

func (c *ControllableBundleServer) Stop() {
	c.proxyServer.Close()
	c.realServer.Stop()
}
