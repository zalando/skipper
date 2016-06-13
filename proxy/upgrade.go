// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	log "github.com/Sirupsen/logrus"
)

// IsUpgradeRequest returns true if and only if there is a "Connection"
// key with the value "Upgrade" in Headers of the given request.
func IsUpgradeRequest(req *http.Request) bool {
	for _, h := range req.Header[http.CanonicalHeaderKey("Connection")] {
		if strings.Contains(strings.ToLower(h), "upgrade") {
			return true
		}
	}
	return false
}

// GetUpgradeRequest returns the protocol name from the upgrade header
func GetUpgradeRequest(req *http.Request) string {
	for _, h := range req.Header[http.CanonicalHeaderKey("Connection")] {
		if strings.Contains(strings.ToLower(h), "upgrade") {
			return strings.Join(req.Header[h], " ")
		}
	}
	return ""
}

// UpgradeProxy stores everything needed to make the connection upgrade.
type UpgradeProxy struct {
	backendAddr  *url.URL
	reverseProxy *httputil.ReverseProxy
	insecure     bool
}

// ServeHTTP inspects the request and either proxies an upgraded connection directly,
// or uses httputil.ReverseProxy to proxy the normal request.
func (p *UpgradeProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	backendConn, err := p.dialBackend(req)
	if err != nil {
		log.Errorf("Error connecting to backend: %s", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service Unavailable"))
		return
	}
	defer backendConn.Close()

	err = req.Write(backendConn)
	if err != nil {
		log.Errorf("Error writing request to backend: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
		return
	}

	// Audit-Log
	_, err = os.Stderr.Write([]byte(fmt.Sprintf("{\"method\": \"%s\", \"path\": \"%s\", \"query\": \"%s\", \"fragment\": \"%s\"}\n", req.Method, req.URL.Path, req.URL.RawQuery, req.URL.Fragment)))
	if err != nil {
		log.Errorf("Could not write audit-log, caused by: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(backendConn), req)
	if err != nil {
		log.Errorf("Error reading response from backend: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
		return
	}

	if resp.StatusCode == http.StatusUnauthorized {
		log.Errorf("Got unauthorized error from backend for: %s %s", req.Method, req.URL)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Internal Server Error, we are not authorized to call the backend."))
		return
	}

	requestHijackedConn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Errorf("Error hijacking request connection: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
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
	// K8s: */attach request is similar to tail -f, which would spam our logs
	if strings.HasSuffix(req.URL.Path, "/attach") {
		copyAsync(&done, backendConn, requestHijackedConn)
	} else {
		copyAsync(&done, backendConn, requestHijackedConn, os.Stdout)
	}
	copyAsync(&done, requestHijackedConn, backendConn)
	log.Debugf("Successfully upgraded to protocol %s by user request", GetUpgradeRequest(req))
	// Wait for goroutine to finish, such that the established connection does not break.
	<-done
}

func (p *UpgradeProxy) dialBackend(req *http.Request) (net.Conn, error) {
	dialAddr := CanonicalAddr(req.URL)

	switch p.backendAddr.Scheme {
	case "http":
		return net.Dial("tcp", dialAddr)
	case "https":
		// TODO(sszuecs): make TLS verification configurable and implement to verify it as default.
		if p.insecure {
			tlsConn, err := tls.Dial("tcp", dialAddr, &tls.Config{InsecureSkipVerify: true})
			if err != nil {
				return nil, err
			}
			return tlsConn, err
		}
		return nil, fmt.Errorf("TLS verification is not implemented, yet")
		// 	hostToVerify, _, err := net.SplitHostPort(dialAddr)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		//err = tlsConn.VerifyHostname(hostToVerify)
	default:
		return nil, fmt.Errorf("unknown scheme: %s", p.backendAddr.Scheme)
	}
}

func copyAsync(c *chan struct{}, src io.Reader, dst ...io.Writer) {
	go func() {
		w := io.MultiWriter(dst...)
		_, err := io.Copy(w, src)
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			log.Errorf("error proxying data from src to dst: %v", err)
		}

		*c <- struct{}{}
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
func CanonicalAddr(url *url.URL) string {
	addr := url.Host
	if !hasPort(addr) {
		return addr + ":" + portMap[url.Scheme]
	}
	return addr
}
