package opatestutils

import (
	"fmt"
	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func StartControllableBundleServer(config BundleServerConfig) *ControllableBundleServer {
	bundleName := config.BundleName
	realSrvs := CreateBundleServers([]string{bundleName})
	cbs := &ControllableBundleServer{
		realServer: realSrvs[bundleName],
		bundleName: bundleName,
	}
	cbs.respCode.Store(config.RespCode)
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

type BundleServerConfig struct {
	BundleName string
	RespCode   int
	Delay      time.Duration
}

// CreateBundleServers creates multiple OPA bundle servers for testing.
// Creates a policy that allows access if input.parsed_path matches the bundle name.
func CreateBundleServers(bundleNames []string) map[string]*opasdktest.Server {
	servers := make(map[string]*opasdktest.Server)
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
		servers[bundleName] = server
	}
	return servers
}

// StopBundleServers stops multiple bundle servers
func (c *MultiBundleProxyController) Stop(proxy *httptest.Server, servers map[string]*opasdktest.Server) {
	proxy.Close()
	for _, srv := range servers {
		srv.Stop()
	}
}

type MultiBundleProxyController struct {
	StatusMap map[string]*atomic.Value
	DelayMap  map[string]*atomic.Value
	Servers   map[string]*opasdktest.Server
}

func (c *MultiBundleProxyController) SetStatus(bundleName string, code int) {
	if v, ok := c.StatusMap[bundleName]; ok {
		v.Store(code)
	}
}
func (c *MultiBundleProxyController) SetDelay(bundleName string, delay time.Duration) {
	if v, ok := c.DelayMap[bundleName]; ok {
		v.Store(delay)
	}
}

// StartMultiBundleProxyServer starts a proxy server that routes requests to multiple controllable bundle servers.
// ToDo clean up
func StartMultiBundleProxyServer(bundleConfigs []BundleServerConfig) (*httptest.Server, *MultiBundleProxyController) {
	bundleNames := make([]string, 0, len(bundleConfigs))
	statusMap := make(map[string]*atomic.Value)
	delayMap := make(map[string]*atomic.Value)
	for _, config := range bundleConfigs {
		bundleNames = append(bundleNames, config.BundleName)
		status := &atomic.Value{}
		status.Store(config.RespCode)
		statusMap[config.BundleName] = status

		delay := &atomic.Value{}
		delay.Store(config.Delay)
		delayMap[config.BundleName] = delay
	}
	servers := CreateBundleServers(bundleNames)
	controller := &MultiBundleProxyController{StatusMap: statusMap, DelayMap: delayMap, Servers: servers}

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bundleName := strings.TrimPrefix(r.URL.Path, "/bundles/")
		realSrv, ok := servers[bundleName]
		status, okStatus := statusMap[bundleName]
		delay, okDelay := delayMap[bundleName]
		if !ok || !okStatus || !okDelay {
			http.NotFound(w, r)
			return
		}
		if d, ok := delay.Load().(time.Duration); ok && d > 0 {
			time.Sleep(d)
		}
		if status.Load().(int) != http.StatusOK {
			w.WriteHeader(status.Load().(int))
			w.Write([]byte("Bundle server error"))
			return
		}
		proxyURL := realSrv.URL() + r.URL.Path
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

	return proxy, controller
}
