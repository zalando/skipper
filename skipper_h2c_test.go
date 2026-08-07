package skipper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zalando/skipper/dataclients/routestring"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

func newH2cListenerBackend(t *testing.T, gotProto *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotProto != nil {
			*gotProto = r.Proto
		}
		w.WriteHeader(http.StatusOK)
	}))
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetHTTP1(true)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// TestH2cEndToEnd_BackendsAndClient verifies end-to-end h2c: an h2c-capable
// backend is reached through a skipper proxy that has EnableH2cServer set.
// Both legs are checked: client→proxy (h2c) and proxy→backend (h2c).
func TestH2cEndToEnd_BackendsAndClient(t *testing.T) {
	if !testListener() {
		t.Skip("skipping listener test; pass -args listener to enable")
	}

	var gotProto string
	backend := newH2cListenerBackend(t, &gotProto)

	dc, err := routestring.New(fmt.Sprintf(`r0: * -> "%s"`, "h2c"+backend.URL[len("http"):]))
	if err != nil {
		t.Fatalf("failed to create data client: %v", err)
	}

	MuFindAddress.Lock()
	addr := FindAddress(t)
	MuFindAddress.Unlock()

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
	})
	defer rt.Close()

	p := proxy.WithParams(proxy.Params{
		Routing: rt,
		Flags:   proxy.Flags(proxy.OptionsNone),
		Metrics: &metricstest.MockMetrics{},
	})
	defer p.Close() //nolint:errcheck

	o := &Options{
		Address:         addr,
		EnableH2cServer: true,
	}

	go listenAndServe(p, o) //nolint:errcheck

	h2cTransport := &http.Transport{}
	h2cTransport.Protocols = new(http.Protocols)
	h2cTransport.Protocols.SetHTTP1(true)
	h2cTransport.Protocols.SetUnencryptedHTTP2(true)
	h2cClient := &http.Client{
		Transport: h2cTransport,
		Timeout:   5 * time.Second,
	}

	var rsp *http.Response
	var reqErr error
	for range 20 {
		rsp, reqErr = h2cClient.Get("http://" + addr + "/")
		if reqErr == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if reqErr != nil {
		t.Fatalf("h2c client request failed: %v", reqErr)
	}
	defer rsp.Body.Close()

	if rsp.Proto != "HTTP/2.0" {
		t.Errorf("expected client-to-proxy h2c proto HTTP/2.0, got %q", rsp.Proto)
	}
	if gotProto != "HTTP/2.0" {
		t.Errorf("expected proxy-to-backend h2c proto HTTP/2.0, got %q", gotProto)
	}
}
