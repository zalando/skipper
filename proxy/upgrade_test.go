package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"golang.org/x/net/websocket"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	for _, ti := range []struct {
		msg                        string
		route                      string
		method                     string
		backendClosesConnection    bool
		backendHangs               bool
		noBackend                  bool
		backendStatusCode          int
		expectedResponseStatusCode int
		expectedResponseBody       string
		backendHeaders             map[string]string
	}{
		{
			msg:               "Load balanced route",
			route:             `route: Path("/ws") -> <roundRobin, "%s">;`,
			method:            http.MethodGet,
			backendStatusCode: http.StatusSwitchingProtocols,
		},
		{
			msg:               "Simple route",
			route:             `route: Path("/ws") -> "%s";`,
			method:            http.MethodGet,
			backendStatusCode: http.StatusSwitchingProtocols,
		},
		{
			msg:               "Wrong method",
			route:             `route: Path("/ws") -> "%s";`,
			method:            http.MethodPost,
			backendStatusCode: http.StatusSwitchingProtocols,
		},
		{
			msg:                        "Closed connection 101",
			route:                      `route: Path("/ws") -> "%s";`,
			method:                     http.MethodGet,
			backendClosesConnection:    true,
			backendStatusCode:          http.StatusSwitchingProtocols,
			expectedResponseStatusCode: http.StatusServiceUnavailable,
		},
		{
			msg:                        "Backend responds 200 with headers",
			route:                      `route: Path("/ws") -> "%s";`,
			method:                     http.MethodGet,
			backendStatusCode:          http.StatusOK,
			backendClosesConnection:    true,
			expectedResponseStatusCode: http.StatusOK,
			backendHeaders:             map[string]string{"X-Header": "value", "Content-Type": "application/json"},
			expectedResponseBody:       "{}",
		},
		{
			msg:                        "Closed connection 204",
			route:                      `route: Path("/ws") -> "%s";`,
			method:                     http.MethodGet,
			backendClosesConnection:    true,
			backendStatusCode:          http.StatusNoContent,
			expectedResponseStatusCode: http.StatusNoContent,
		},
		{
			msg:                        "No backend",
			route:                      `route: Path("/ws") -> "%s";`,
			method:                     http.MethodGet,
			noBackend:                  true,
			backendStatusCode:          http.StatusSwitchingProtocols,
			expectedResponseStatusCode: http.StatusServiceUnavailable,
		},
		{
			msg:                        "backend reject upgrade",
			route:                      `route: Path("/ws") -> "%s";`,
			method:                     http.MethodPost,
			backendClosesConnection:    true,
			backendStatusCode:          http.StatusBadRequest,
			expectedResponseStatusCode: http.StatusBadRequest,
			expectedResponseBody:       "BACKEND ERROR",
		},
		{
			msg:               "backend hangs",
			route:             `route: Path("/ws") -> "%s";`,
			method:            http.MethodGet,
			backendStatusCode: http.StatusSwitchingProtocols,
			backendHangs:      true,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			ti := ti // trick race detector
			var clientConnClosed atomic.Bool

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if ti.backendHeaders != nil {
					for k, v := range ti.backendHeaders {
						w.Header().Set(k, v)
					}
				}
				if ti.backendClosesConnection {
					// Set header first as 1xx headers are sent immediately by w.WriteHeader
					w.Header().Set("Connection", "close")
					w.WriteHeader(ti.backendStatusCode)
					if len(ti.expectedResponseBody) > 0 {
						w.Write([]byte(ti.expectedResponseBody))
					}
					return
				}

				w.WriteHeader(ti.backendStatusCode)

				hj, ok := w.(http.Hijacker)
				require.True(t, ok, "webserver doesn't support hijacking")

				conn, bufrw, err := hj.Hijack()
				require.NoError(t, err)

				// Closing server does not close hijacked connections so do it explicitly
				defer conn.Close()

				for {
					s, err := bufrw.ReadString('\n')
					if err != nil {
						if !clientConnClosed.Load() {
							t.Error(err)
						}
						return
					}

					if ti.backendHangs {
						return // will close connection without response
					}

					var resp string
					if strings.Compare(s, "ping\n") == 0 {
						resp = "pong\n"
					} else {
						resp = "bad\n"
					}

					_, err = bufrw.WriteString(resp)
					if err != nil {
						if !clientConnClosed.Load() {
							t.Error(err)
						}
						return
					}
					err = bufrw.Flush()
					if err != nil {
						if !clientConnClosed.Load() {
							t.Error(err)
						}
						return
					}
				}
			}))

			routes := fmt.Sprintf(ti.route, backend.URL)

			if ti.noBackend {
				backend.Close()
			} else {
				defer backend.Close()
			}

			tp, err := newTestProxyWithParams(routes, Params{ExperimentalUpgrade: true})
			require.NoError(t, err)

			defer tp.close()

			skipper := httptest.NewServer(tp.proxy)
			defer skipper.Close()

			skipperUrl, _ := url.Parse(skipper.URL)
			conn, err := net.Dial("tcp", skipperUrl.Host)
			require.NoError(t, err)

			defer func() {
				clientConnClosed.Store(true)
				conn.Close()
			}()

			u, _ := url.ParseRequestURI("wss://www.example.org/ws")
			r := &http.Request{
				URL:    u,
				Method: ti.method,
				Header: http.Header{
					"Connection": []string{"Upgrade"},
					"Upgrade":    []string{"websocket"},
				},
			}
			err = r.Write(conn)
			require.NoError(t, err)

			reader := bufio.NewReader(conn)
			resp, err := http.ReadResponse(reader, r)
			require.NoError(t, err)

			if resp.StatusCode != http.StatusSwitchingProtocols {
				assert.Equal(t, ti.expectedResponseStatusCode, resp.StatusCode)

				if ti.expectedResponseBody != "" {
					data, err := io.ReadAll(resp.Body)
					require.NoError(t, err)

					assert.Equal(t, ti.expectedResponseBody, string(data))
				}
				if ti.backendHeaders != nil {
					for k, v := range ti.backendHeaders {
						assert.Equal(t, v, resp.Header.Get(k), "Should copy all headers from response")
					}
				}

				return
			}

			_, err = conn.Write([]byte("ping\n"))
			require.NoError(t, err)

			pong, err := reader.ReadString('\n')
			if ti.backendHangs {
				assert.Equal(t, io.EOF, err, "expected EOF on closed connection read")
			} else {
				require.NoError(t, err)
				assert.Equal(t, "pong\n", pong)
			}
		})
	}
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

func TestAuditLogging(t *testing.T) {
	message := strconv.Itoa(rand.Int())
	test := func(enabled bool, check func(*testing.T, string, string)) func(t *testing.T) {
		return func(t *testing.T) {
			wss := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
				if _, err := io.Copy(ws, ws); err != nil {
					t.Error(err)
				}
			}))
			defer wss.Close()

			// only used as poor man's sync, the audit log in question goes stdout and stderr,
			// see below
			tl := loggingtest.New()
			defer tl.Close()

			dc := testdataclient.New([]*eskip.Route{{Backend: wss.URL}})
			defer dc.Close()

			rt := routing.New(routing.Options{
				DataClients: []routing.DataClient{dc},
				Log:         tl,
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

			check(t, sout.String(), serr.String())
		}
	}

	t.Run("off", test(false, func(t *testing.T, sout, serr string) {
		if sout != "" || len(serr) != 0 {
			t.Errorf("failed to disable audit log: %s", sout)
		}
	}))

	t.Run("on", test(true, func(t *testing.T, sout, serr string) {
		if !strings.Contains(sout, message) || len(serr) == 0 {
			t.Error("failed to enable audit log")
		}
	}))
}
