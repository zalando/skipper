package dnstest

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func withHostname(addr, host string) string {
	u, _ := url.Parse(addr)
	u.Host = net.JoinHostPort(host, u.Port())
	return u.String()
}

func TestLocalHostnames(t *testing.T) {
	const alias = "bar.foo.test"

	LoopbackNames(t, alias)

	requestHostname := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHostname, _, _ = net.SplitHostPort(r.Host)
	}))
	defer ts.Close()

	res, err := http.Get(withHostname(ts.URL, alias))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if requestHostname != alias {
		t.Errorf("expected: %s, got: %s", alias, requestHostname)
	}
}
