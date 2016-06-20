package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"
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
