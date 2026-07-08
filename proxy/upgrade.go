package proxy

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// defaultDialTimeout is the maximum time allowed to establish a TCP (or TLS)
// connection to an upgrade backend. It acts as a hard ceiling that bounds
// goroutine lifetime even when the caller's context carries no deadline,
// preventing unbounded goroutine accumulation on stalled backends.
const defaultDialTimeout = 30 * time.Second

// isUpgradeRequest returns true if and only if there is a "Connection"
// key with the value "Upgrade" in Headers of the given request.
func isUpgradeRequest(req *http.Request) bool {
	for _, h := range req.Header[http.CanonicalHeaderKey("Connection")] {
		if strings.Contains(strings.ToLower(h), "upgrade") {
			return true
		}
	}
	return false
}

// getUpgradeRequest returns the protocol name from the upgrade header
func getUpgradeRequest(req *http.Request) string {
	for _, h := range req.Header[http.CanonicalHeaderKey("Connection")] {
		if strings.Contains(strings.ToLower(h), "upgrade") {
			return strings.Join(req.Header.Values(h), " ")
		}
	}
	return ""
}

// UpgradeProxy stores everything needed to make the connection upgrade.
type upgradeProxy struct {
	backendAddr     *url.URL
	reverseProxy    *httputil.ReverseProxy
	tlsClientConfig *tls.Config
	insecure        bool
	useAuditLog     bool
	auditLogOut     io.Writer
	auditLogErr     io.Writer
	auditLogHook    chan struct{}
}

// TODO: add user here
type auditLog struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Query    string `json:"query"`
	Fragment string `json:"fragment"`
}

// serveHTTP establishes a bidirectional connection, creates an
// auditlog for the request target, copies the data back and force and
// write data to an auditlog. It will not return until the connection
// is closed.
func (p *upgradeProxy) serveHTTP(w http.ResponseWriter, req *http.Request) {
	// The following check is based on
	// https://tools.ietf.org/html/rfc2616#section-14.42
	// https://tools.ietf.org/html/rfc7230#section-6.7
	// and https://tools.ietf.org/html/rfc6455 (websocket)
	if (req.ProtoMajor <= 1 && req.ProtoMinor < 1) ||
		!isUpgradeRequest(req) ||
		req.Header.Get("Upgrade") == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(http.StatusText(http.StatusBadRequest)))
		return
	}

	backendConn, err := p.dialBackend(req)
	if err != nil {
		log.Errorf("Error connecting to backend: %s", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(http.StatusText(http.StatusServiceUnavailable)))
		return
	}
	defer backendConn.Close()

	err = req.Write(backendConn)
	if err != nil {
		log.Errorf("Error writing request to backend: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(http.StatusText(http.StatusInternalServerError)))
		return
	}

	// Audit-Log
	if p.useAuditLog {
		auditlog := &auditLog{req.Method, req.URL.Path, req.URL.RawQuery, req.URL.Fragment}
		auditJSON, err := json.Marshal(auditlog)
		if err == nil {
			_, err = p.auditLogErr.Write(auditJSON)
		}
		if err != nil {
			log.Errorf("Could not write audit-log, caused by: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(http.StatusText(http.StatusInternalServerError)))
			return
		}
	}

	resp, err := http.ReadResponse(bufio.NewReader(backendConn), req)
	if err != nil {
		log.Errorf("Error reading response from backend: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(http.StatusText(http.StatusInternalServerError)))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		log.Debugf("Got unauthorized error from backend for: %s %s", req.Method, req.URL)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(http.StatusText(http.StatusUnauthorized)))
		return
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		log.Debugf("Got invalid status code from backend: %d", resp.StatusCode)
		maps.Copy(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, err := io.Copy(w, resp.Body)
		if err != nil {
			log.Errorf("Error writing body to client: %s", err)
			return
		}
		return
	}

	// Backend sent Connection: close
	if resp.Close {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(http.StatusText(http.StatusServiceUnavailable)))
		return
	}

	requestHijackedConn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Errorf("Error hijacking request connection: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(http.StatusText(http.StatusInternalServerError)))
		return
	}

	defer requestHijackedConn.Close()
	// NOTE: from this point forward, we own the connection and we can't use
	// w.Header(), w.Write(), or w.WriteHeader any more

	err = resp.Write(requestHijackedConn)
	if err != nil {
		log.Errorf("Error writing backend response to client: %s", err)
		return
	}

	done := make(chan struct{}, 2)

	if p.useAuditLog {
		copyAsync("backend->request+audit", backendConn, io.MultiWriter(p.auditLogOut, requestHijackedConn), done)
	} else {
		copyAsync("backend->request", backendConn, requestHijackedConn, done)
	}

	copyAsync("request->backend", requestHijackedConn, backendConn, done)

	log.Debugf("Successfully upgraded to protocol %s by user request", getUpgradeRequest(req))

	// Wait for either copyAsync to complete.
	// Return from this method closes both request and backend connections via defer
	// and thus unblocks the second copyAsync.
	<-done

	if p.useAuditLog {
		select {
		case p.auditLogHook <- struct{}{}:
		default:
		}
	}
}

// dialBackend opens a TCP connection to the upgrade backend.
//
// Availability fix: the original net.Dial / tls.Dial calls had no timeout and
// no context propagation, meaning a stalled or slow backend could keep the
// goroutine (and its file descriptor) alive indefinitely.  Under high
// concurrency this causes unbounded goroutine accumulation and FD exhaustion.
//
// The fix introduces two complementary bounds:
//  1. defaultDialTimeout (30 s) — a hard ceiling that fires even when the
//     caller's context carries no deadline.
//  2. req.Context() — honours client-side cancellation / request deadlines so
//     the dial aborts as soon as the upstream request is gone.
//
// Both bounds are passed to DialContext; whichever fires first wins.
func (p *upgradeProxy) dialBackend(req *http.Request) (net.Conn, error) {
	dialAddr := canonicalAddr(req.URL)

	switch p.backendAddr.Scheme {
	case "http":
		// DialContext propagates the request context AND the 30-second hard
		// ceiling, replacing the unbounded net.Dial("tcp", dialAddr) call.
		return (&net.Dialer{Timeout: defaultDialTimeout}).DialContext(req.Context(), "tcp", dialAddr)

	case "https":
		// tls.Dialer wraps a timed net.Dialer so both the TLS handshake and
		// the underlying TCP connect are subject to defaultDialTimeout and the
		// request context, replacing the unbounded tls.Dial call.
		tlsDialer := &tls.Dialer{
			NetDialer: &net.Dialer{Timeout: defaultDialTimeout},
			Config:    p.tlsClientConfig,
		}
		conn, err := tlsDialer.DialContext(req.Context(), "tcp", dialAddr)
		if err != nil {
			return nil, err
		}

		if !p.insecure {
			hostToVerify, _, err := net.SplitHostPort(dialAddr)
			if err != nil {
				conn.Close()
				return nil, err
			}
			// tls.Dialer.DialContext returns net.Conn; assert to *tls.Conn
			// for post-handshake hostname verification.
			if err = conn.(*tls.Conn).VerifyHostname(hostToVerify); err != nil {
				conn.Close()
				return nil, err
			}
		}

		return conn, nil

	default:
		return nil, fmt.Errorf("unknown scheme: %s", p.backendAddr.Scheme)
	}
}

func copyAsync(dir string, src io.Reader, dst io.Writer, done chan<- struct{}) {
	go func() {
		_, err := io.Copy(dst, src)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			log.Errorf("error copying data %s: %v", dir, err)
		}
		done <- struct{}{}
	}()
}

// FROM: http://golang.org/src/net/http/client.go
// Given a string of the form "host", "host:port", or "[ipv6::address]:port",
// return true if the string includes a port.
func hasPort(s string) bool { return strings.LastIndex(s, ":") > strings.LastIndex(s, "]") }

// FROM: http://golang.org/src/net/http/transport.go
var portMap = map[string]string{
	"http":  "80",
	"https": "443",
}

// FROM: http://golang.org/src/net/http/transport.go
// canonicalAddr returns url.Host but always with a ":port" suffix
func canonicalAddr(url *url.URL) string {
	addr := url.Host
	if !hasPort(addr) {
		return addr + ":" + portMap[url.Scheme]
	}
	return addr
}
