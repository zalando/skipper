package proxy

import (
	"bytes"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"golang.org/x/net/websocket"
)

func getEmptyUpgradeRequest() *http.Request {
	return &http.Request{
		Header: http.Header{},
	}
}

func getInvalidUpgradeRequest() *http.Request {
	header := http.Header{}
	header.Add("Connection", "Upgrade")
	return &http.Request{
		Header: header,
	}
}

func getValidUpgradeRequest() (*http.Request, string) {
	//Connection:[Upgrade] Upgrade:[SPDY/3.1]
	//prot := "HTTP/2.0, SPDY/3.1"
	prot := "SPDY/3.1"
	header := http.Header{}
	header.Add("Connection", "Upgrade")
	header.Add("Upgrade", prot)
	return &http.Request{
		Header: header,
	}, prot
}

func TestEmptyGetUpgradeRequest(t *testing.T) {
	req := getEmptyUpgradeRequest()
	if isUpgradeRequest(req) {
		t.Errorf("Request has no upgrade header, but isUpgradeRequest returned true for %+v", req)
	}
	if getUpgradeRequest(req) != "" {
		t.Errorf("Request has no upgrade header, but getUpgradeRequest returned not emptystring for %+v", req)
	}

}

func TestInvalidGetUpgradeRequest(t *testing.T) {
	req := getInvalidUpgradeRequest()
	if !isUpgradeRequest(req) {
		t.Errorf("Request has a connection upgrade header, but no upgrade header. isUpgradeRequest should return true for %+v", req)
	}
	if getUpgradeRequest(req) != "" {
		t.Errorf("Request has no upgrade header, but getUpgradeRequest returned not emptystring for %+v", req)
	}

}

func TestValidGetUpgradeRequest(t *testing.T) {
	req, prot := getValidUpgradeRequest()
	if !isUpgradeRequest(req) {
		t.Errorf("Request has an upgrade header, but isUpgradeRequest returned false for %+v", req)
	}
	gotProt := getUpgradeRequest(req)
	if gotProt != prot {
		t.Errorf("%s != %s for %+v", gotProt, prot, req)
	}

}

func TestServeHTTP(t *testing.T) {
	// TODO
}

func getReverseProxy(backendURL *url.URL) *httputil.ReverseProxy {
	reverseProxy := httputil.NewSingleHostReverseProxy(backendURL)
	reverseProxy.FlushInterval = 20 * time.Millisecond
	return reverseProxy
}

func getUpgradeProxy() *upgradeProxy {
	u, _ := url.ParseRequestURI("http://127.0.0.1:8080/foo")
	return &upgradeProxy{
		backendAddr:  u,
		reverseProxy: getReverseProxy(u),
		insecure:     false,
	}
}

func getHTTPRequest(urlStr string) (*http.Request, error) {
	return http.NewRequest("http", urlStr, nil)
}

func TestHTTPDialBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "test-value")
	}))
	defer server.Close()

	p := getUpgradeProxy()
	req, err := getHTTPRequest(server.URL)
	if err != nil {
		t.Errorf("getHTTPRequest returns an error: %v", err)
	}

	_, err = p.dialBackend(req)
	if err != nil {
		t.Errorf("Could not dial to %s, caused by: %v", req.Host, err)
	}
}

func TestInvalidHTTPDialBackend(t *testing.T) {
	p := getUpgradeProxy()
	req, err := getHTTPRequest("ftp://localhost/foo")
	if err != nil {
		t.Errorf("getHTTPRequest returns an error: %v", err)
	}

	_, err = p.dialBackend(req)
	if err == nil {
		t.Errorf("Could dial to %s, but should not be possible, caused by: %v", req.Host, err)
	}
}

func TestCopyAsync(t *testing.T) {
	var dst bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	s := "foo"
	src := bytes.NewBufferString(s)

	copyAsync(&wg, src, &dst)
	wg.Wait()
	res := dst.String()
	if res != s {
		t.Errorf("%s != %s after copy", res, s)
	}
}

func TestAuditLogging(t *testing.T) {
	message := strconv.Itoa(rand.Int())
	test := func(enabled bool, check func(*testing.T, *bytes.Buffer, *bytes.Buffer)) func(t *testing.T) {
		return func(t *testing.T) {
			wss := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
				if _, err := io.Copy(ws, ws); err != nil {
					t.Fatal(err)
				}
			}))

			defer wss.Close()

			// only used as poor man's sync, the audit log in question goes stdout and stderr,
			// see below
			tl := loggingtest.New()
			rt := routing.New(routing.Options{
				DataClients: []routing.DataClient{
					testdataclient.New([]*eskip.Route{{Backend: wss.URL}}),
				},
				Log: tl,
			})
			defer rt.Close()

			if err := tl.WaitFor("route settings applied", 120*time.Millisecond); err != nil {
				t.Fatal(err)
			}

			auditHook := make(chan struct{}, 1)
			p := WithParams(Params{
				Routing:                  rt,
				ExperimentalUpgrade:      true,
				ExperimentalUpgradeAudit: enabled,
			})
			p.auditLogHook = auditHook
			defer p.Close()

			ps := httptest.NewServer(p)
			defer ps.Close()

			sout := bytes.NewBuffer(nil)
			serr := bytes.NewBuffer(nil)
			p.upgradeAuditLogOut = sout
			p.upgradeAuditLogErr = serr

			wsc, err := websocket.Dial(
				strings.Replace(ps.URL, "http:", "ws:", 1),
				"",
				"http://[::1]",
			)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := wsc.Write([]byte(message)); err != nil {
				t.Fatal(err)
			}

			receive := make([]byte, len(message))
			if _, err := wsc.Read(receive); err != nil {
				t.Fatal(err)
			}

			if string(receive) != message {
				t.Fatal("send/receive failed")
			}

			wsc.Close()
			if enabled {
				<-p.auditLogHook
			}

			check(t, sout, serr)
		}
	}

	t.Run("off", test(false, func(t *testing.T, sout, serr *bytes.Buffer) {
		if sout.Len() != 0 || serr.Len() != 0 {
			t.Error("failed to disable audit log")
		}
	}))

	t.Run("on", test(true, func(t *testing.T, sout, serr *bytes.Buffer) {
		if !strings.Contains(sout.String(), message) || serr.Len() == 0 {
			t.Error("failed to enable audit log")
		}
	}))
}
