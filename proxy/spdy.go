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
	"strings"

	log "github.com/Sirupsen/logrus"
)

var username string = ""
var password string = ""

// var frontend string
// var backend string

// func CreateSpdyProxy() {
// 	u1, _ := url.Parse(frontend)
// 	u2, _ := url.Parse(backend)

// 	proxy, err := NewUpgradeAwareSingleHostReverseProxy(u1, u2)
// 	if err != nil {
// 		log.Fatalf("Unable to initialize the Kubernetes proxy: %v", err)
// 	}
// 	log.Fatal(http.ListenAndServe(":7070", proxy))
// }

// start NEEDED
func isUpgradeRequest(req *http.Request) bool {
	for _, h := range req.Header[http.CanonicalHeaderKey("Connection")] {
		if strings.Contains(strings.ToLower(h), "upgrade") {
			return true
		}
	}
	return false
}

type SpdyProxy struct {
	backendAddr  *url.URL
	reverseProxy *httputil.ReverseProxy
}

// ServeHTTP inspects the request and either proxies an upgraded connection directly,
// or uses httputil.ReverseProxy to proxy the normal request.
func (p *SpdyProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	newReq, err := p.newProxyRequest(req)
	if err != nil {
		log.Printf("Error creating backend request: %s", err)
		// TODO do we need to do more than this?
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	p.serveUpgrade(w, newReq)
}

func (p *SpdyProxy) newProxyRequest(req *http.Request) (*http.Request, error) {
	// TODO is this the best way to clone the original request and create
	// the new request for the backend? Do we need to copy anything else?
	//
	backendURL := *p.backendAddr
	// if backendAddr is http://host/base and req is for /foo, the resulting path
	// for backendURL should be /base/foo
	backendURL.Path = singleJoiningSlash(backendURL.Path, req.URL.Path)
	backendURL.RawQuery = req.URL.RawQuery

	newReq, err := http.NewRequest(req.Method, backendURL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	// TODO is this the right way to copy headers?
	newReq.Header = req.Header

	// TODO do we need to exclude any other headers?
	removeAuthHeaders(newReq)

	return newReq, nil
}

func (p *SpdyProxy) dialBackend(req *http.Request) (net.Conn, error) {
	dialAddr := CanonicalAddr(req.URL)

	switch p.backendAddr.Scheme {
	case "http":
		return net.Dial("tcp", dialAddr)
	case "https":
		//	return tls.Dial("tcp", dialAddr, &tls.Config{InsecureSkipVerify: true})
		// 	tlsConfig, err := restclient.TLSConfigFor(p.clientConfig)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		tlsConn, err := tls.Dial("tcp", dialAddr, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return nil, err
		}
		// 	hostToVerify, _, err := net.SplitHostPort(dialAddr)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	err = tlsConn.VerifyHostname(hostToVerify)
		return tlsConn, err
	default:
		return nil, fmt.Errorf("unknown scheme: %s", p.backendAddr.Scheme)
	}
}

func (p *SpdyProxy) serveUpgrade(w http.ResponseWriter, req *http.Request) {
	backendConn, err := p.dialBackend(req)
	if err != nil {
		log.Printf("Error connecting to backend: %s", err)
		// TODO do we need to do more than this?
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	defer backendConn.Close()

	addAuthHeaders(req)

	err = req.Write(backendConn)
	if err != nil {
		log.Printf("Error writing request to backend: %s", err)
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(backendConn), req)
	if err != nil {
		log.Printf("Error reading response from backend: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
		return
	}

	if resp.StatusCode == http.StatusUnauthorized {
		log.Printf("Got unauthorized error from backend for: %s %s", req.Method, req.URL)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
		return
	}

	requestHijackedConn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Printf("Error hijacking request connection: %s", err)
		return
	}
	defer requestHijackedConn.Close()

	// NOTE: from this point forward, we own the connection and we can't use
	// w.Header(), w.Write(), or w.WriteHeader any more

	removeCORSHeaders(resp)
	removeChallengeHeaders(resp)
	err = resp.Write(requestHijackedConn)
	if err != nil {
		log.Printf("Error writing backend response to client: %s", err)
		return
	}

	done := make(chan struct{}, 2)

	go func() {
		_, err := io.Copy(backendConn, requestHijackedConn)
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			log.Printf("error proxying data from client to backend: %v", err)
		}
		done <- struct{}{}
	}()

	go func() {
		_, err := io.Copy(requestHijackedConn, backendConn)
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			log.Printf("error proxying data from backend to client: %v", err)
		}
		done <- struct{}{}
	}()

	<-done
}

// end NEEDED

// // UpgradeAwareSingleHostReverseProxy is capable of proxying both regular HTTP
// // connections and those that require upgrading (e.g. web sockets). It implements
// // the http.RoundTripper and http.Handler interfaces.
// type UpgradeAwareSingleHostReverseProxy struct {
// 	frontendAddr *url.URL
// 	backendAddr  *url.URL
// 	transport    http.RoundTripper
// 	reverseProxy *httputil.ReverseProxy
// }

// // NewUpgradeAwareSingleHostReverseProxy creates a new UpgradeAwareSingleHostReverseProxy.
// func NewUpgradeAwareSingleHostReverseProxy(frontendAddr *url.URL, backendAddr *url.URL) (*UpgradeAwareSingleHostReverseProxy, error) {
// 	transport := http.DefaultTransport.(*http.Transport)
// 	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

// 	reverseProxy := httputil.NewSingleHostReverseProxy(backendAddr)
// 	reverseProxy.FlushInterval = 200 * time.Millisecond
// 	p := &UpgradeAwareSingleHostReverseProxy{
// 		frontendAddr: frontendAddr,
// 		backendAddr:  backendAddr,
// 		transport:    transport,
// 		reverseProxy: reverseProxy,
// 	}
// 	p.reverseProxy.Transport = p
// 	return p, nil
// }

// // RoundTrip sends the request to the backend and strips off the CORS headers
// // before returning the response.
// func (p *UpgradeAwareSingleHostReverseProxy) RoundTrip(req *http.Request) (*http.Response, error) {
// 	resp, err := p.transport.RoundTrip(req)
// 	if err != nil {
// 		return resp, err
// 	}

// 	removeCORSHeaders(resp)
// 	removeChallengeHeaders(resp)
// 	if resp.StatusCode == http.StatusUnauthorized {
// 		log.Printf("got unauthorized error from backend for: %s %s", req.Method, req.URL)
// 		// Internal error, backend didn't recognize proxy identity
// 		// Surface as a server error to the client
// 		// TODO do we need to do more than this?
// 		resp = &http.Response{
// 			StatusCode:    http.StatusInternalServerError,
// 			Status:        http.StatusText(http.StatusInternalServerError),
// 			Body:          ioutil.NopCloser(strings.NewReader("Internal Server Error")),
// 			ContentLength: -1,
// 		}
// 	}

// 	// TODO do we need to strip off anything else?

// 	return resp, err
// }

// func (p *UpgradeAwareSingleHostReverseProxy) newProxyRequest(req *http.Request) (*http.Request, error) {
// 	// TODO is this the best way to clone the original request and create
// 	// the new request for the backend? Do we need to copy anything else?
// 	//
// 	backendURL := *p.backendAddr
// 	// if backendAddr is http://host/base and req is for /foo, the resulting path
// 	// for backendURL should be /base/foo
// 	backendURL.Path = singleJoiningSlash(backendURL.Path, req.URL.Path)
// 	backendURL.RawQuery = req.URL.RawQuery

// 	newReq, err := http.NewRequest(req.Method, backendURL.String(), req.Body)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// TODO is this the right way to copy headers?
// 	newReq.Header = req.Header

// 	// TODO do we need to exclude any other headers?
// 	removeAuthHeaders(newReq)

// 	return newReq, nil
// }

// // ServeHTTP inspects the request and either proxies an upgraded connection directly,
// // or uses httputil.ReverseProxy to proxy the normal request.
// func (p *UpgradeAwareSingleHostReverseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
// 	newReq, err := p.newProxyRequest(req)
// 	if err != nil {
// 		log.Printf("Error creating backend request: %s", err)
// 		// TODO do we need to do more than this?
// 		w.WriteHeader(http.StatusInternalServerError)
// 		return
// 	}

// 	if !isUpgradeRequest(req) {
// 		p.reverseProxy.ServeHTTP(w, newReq)
// 		return
// 	}

// 	p.serveUpgrade(w, newReq)
// }

// func (p *UpgradeAwareSingleHostReverseProxy) dialBackend(req *http.Request) (net.Conn, error) {
// 	dialAddr := CanonicalAddr(req.URL)

// 	switch p.backendAddr.Scheme {
// 	case "http":
// 		return net.Dial("tcp", dialAddr)
// 	case "https":
// 		//	return tls.Dial("tcp", dialAddr, &tls.Config{InsecureSkipVerify: true})
// 		// 	tlsConfig, err := restclient.TLSConfigFor(p.clientConfig)
// 		// 	if err != nil {
// 		// 		return nil, err
// 		// 	}
// 		tlsConn, err := tls.Dial("tcp", dialAddr, &tls.Config{InsecureSkipVerify: true})
// 		if err != nil {
// 			return nil, err
// 		}
// 		// 	hostToVerify, _, err := net.SplitHostPort(dialAddr)
// 		// 	if err != nil {
// 		// 		return nil, err
// 		// 	}
// 		// 	err = tlsConn.VerifyHostname(hostToVerify)
// 		return tlsConn, err
// 	default:
// 		return nil, fmt.Errorf("unknown scheme: %s", p.backendAddr.Scheme)
// 	}
// }

// func (p *UpgradeAwareSingleHostReverseProxy) serveUpgrade(w http.ResponseWriter, req *http.Request) {
// 	backendConn, err := p.dialBackend(req)
// 	if err != nil {
// 		log.Printf("Error connecting to backend: %s", err)
// 		// TODO do we need to do more than this?
// 		w.WriteHeader(http.StatusServiceUnavailable)
// 		return
// 	}
// 	defer backendConn.Close()

// 	addAuthHeaders(req)

// 	err = req.Write(backendConn)
// 	if err != nil {
// 		log.Printf("Error writing request to backend: %s", err)
// 		return
// 	}

// 	resp, err := http.ReadResponse(bufio.NewReader(backendConn), req)
// 	if err != nil {
// 		log.Printf("Error reading response from backend: %s", err)
// 		w.WriteHeader(http.StatusInternalServerError)
// 		w.Write([]byte("Internal Server Error"))
// 		return
// 	}

// 	if resp.StatusCode == http.StatusUnauthorized {
// 		log.Printf("Got unauthorized error from backend for: %s %s", req.Method, req.URL)
// 		w.WriteHeader(http.StatusInternalServerError)
// 		w.Write([]byte("Internal Server Error"))
// 		return
// 	}

// 	requestHijackedConn, _, err := w.(http.Hijacker).Hijack()
// 	if err != nil {
// 		log.Printf("Error hijacking request connection: %s", err)
// 		return
// 	}
// 	defer requestHijackedConn.Close()

// 	// NOTE: from this point forward, we own the connection and we can't use
// 	// w.Header(), w.Write(), or w.WriteHeader any more

// 	removeCORSHeaders(resp)
// 	removeChallengeHeaders(resp)
// 	err = resp.Write(requestHijackedConn)
// 	if err != nil {
// 		log.Printf("Error writing backend response to client: %s", err)
// 		return
// 	}

// 	done := make(chan struct{}, 2)

// 	go func() {
// 		_, err := io.Copy(backendConn, requestHijackedConn)
// 		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
// 			// utilruntime.HandleError(("error proxying data from client to backend: %v", err))
// 			log.Printf("error proxying data from client to backend: %v", err)
// 		}
// 		done <- struct{}{}
// 	}()

// 	go func() {
// 		_, err := io.Copy(requestHijackedConn, backendConn)
// 		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
// 			// utilruntime.HandleError(("error proxying data from backend to client: %v", err))
// 			log.Printf("error proxying data from backend to client: %v", err)
// 		}
// 		done <- struct{}{}
// 	}()

// 	<-done
// }

// removeAuthHeaders strips authorization headers from an incoming client
// This should be called on all requests before proxying
func removeAuthHeaders(req *http.Request) {
	req.Header.Del("Authorization")
}

// removeChallengeHeaders strips WWW-Authenticate headers from backend responses
// This should be called on all responses before returning
func removeChallengeHeaders(resp *http.Response) {
	resp.Header.Del("WWW-Authenticate")
}

// removeCORSHeaders strip CORS headers sent from the backend
// This should be called on all responses before returning
func removeCORSHeaders(resp *http.Response) {
	resp.Header.Del("Access-Control-Allow-Credentials")
	resp.Header.Del("Access-Control-Allow-Headers")
	resp.Header.Del("Access-Control-Allow-Methods")
	resp.Header.Del("Access-Control-Allow-Origin")
}

// addAuthHeaders adds basic auth from the given config (if specified)
// This should be run on any requests not handled by the transport returned from TransportFor(config)
func addAuthHeaders(req *http.Request) {
	req.SetBasicAuth(username, password)
}

// FROM: http://golang.org/src/net/http/httputil/reverseproxy.go
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
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
