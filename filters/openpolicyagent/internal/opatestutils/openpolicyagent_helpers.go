package opatestutils

import (
	"fmt"
	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"
)

// ControllableBundleServer - A bundle server whose response code and response latency can be controlled for testing
type ControllableBundleServer struct {
	realServer  *opasdktest.Server
	proxyServer *httptest.Server
	respCode    atomic.Value // stores http status code (int)
	delay       atomic.Value // stores time.Duration - artificial delay to apply to each bundle request before responding
	bundleName  string
}

func StartControllableBundleServer(bundleName string, respCode int) *ControllableBundleServer {
	realSrv := CreateBundleServers([]string{bundleName})[0]
	cbs := &ControllableBundleServer{
		realServer: realSrv,
		bundleName: bundleName,
	}
	cbs.respCode.Store(respCode)
	cbs.delay.Store(time.Duration(0))

	cbs.proxyServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay, ok := cbs.delay.Load().(time.Duration); ok && delay > 0 {
			time.Sleep(delay)
		}

		if cbs.respCode.Load().(int) != http.StatusOK {
			w.WriteHeader(cbs.respCode.Load().(int))
			w.Write([]byte("Bundle server error"))
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

func (c *ControllableBundleServer) SetRespCode(respCode int) {
	c.respCode.Store(respCode)
}

func (c *ControllableBundleServer) SetDelay(delay time.Duration) {
	c.delay.Store(delay)
}

func (c *ControllableBundleServer) URL() string {
	return c.proxyServer.URL
}

func (c *ControllableBundleServer) Stop() {
	c.proxyServer.Close()
	c.realServer.Stop()
}

// StartMultipleControllableBundleServers - Helper to create multiple bundle servers
func StartMultipleControllableBundleServers(bundleConfigs []BundleServerConfig) []*ControllableBundleServer {
	var servers []*ControllableBundleServer
	for _, config := range bundleConfigs {
		server := StartControllableBundleServer(config.BundleName, config.RespCode)
		if config.Delay > 0 {
			server.SetDelay(config.Delay)
		}
		servers = append(servers, server)
	}
	return servers
}

type BundleServerConfig struct {
	BundleName string
	RespCode   int
	Delay      time.Duration
}

// CreateBundleServers creates multiple OPA bundle servers for testing.
// Creates a policy that allows access if input.parsed_path matches the bundle name.
func CreateBundleServers(bundleNames []string) []*opasdktest.Server {
	var servers []*opasdktest.Server
	for i, bundleName := range bundleNames {
		packageName := fmt.Sprintf("test%d", i+1)
		server := opasdktest.MustNewServer(
			opasdktest.MockBundle("/bundles/"+bundleName, map[string]string{
				"main.rego": fmt.Sprintf(`
					package %s
					import rego.v1
					default allow := false

					allow if {
						input.parsed_path == ["%s"]
					}
				`, packageName, bundleName),
				".manifest": fmt.Sprintf(`{
					"roots": ["%s"]
				}`, packageName),
			}),
		)
		servers = append(servers, server)
	}
	return servers
}

// StopBundleServers stops multiple bundle servers
func StopBundleServers(servers []*opasdktest.Server) {
	for _, server := range servers {
		server.Stop()
	}
}
