package proxytest_test

import (
	"crypto/tls"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/net/dnstest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
)

func TestNew(t *testing.T) {
	fr := builtin.MakeRegistry()
	routes := eskip.MustParse(`r: * -> status(200) -> inlineContent("ok") -> <shunt>;`)
	p := proxytest.New(fr, routes...)
	defer p.Close()

	rsp, body, err := p.Client().GetBody(p.URL)
	if err != nil {
		t.Fatalf("Failed to get response: %v", err)
	}
	if rsp.StatusCode != 200 {
		t.Fatalf("Failed to get response status 200, got: %d", rsp.StatusCode)
	}
	if s := string(body); s != "ok" {
		t.Fatalf(`Failed to get response body "ok", got: %q`, s)
	}

}

func TestHttps(t *testing.T) {
	dnstest.LoopbackNames(t, "foo.skipper.test", "bar.skipper.test")

	p := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
		},
		Routes: eskip.MustParse(`
			any: * -> inlineContent("any host response") -> <shunt>;
			foo: Host("^foo[.]skipper[.]test") -> inlineContent("foo.skipper.test response") -> <shunt>;
			bar: Host("^bar[.]skipper[.]test") -> inlineContent("bar.skipper.test response") -> <shunt>;
		`),
		Certificates: []tls.Certificate{
			proxytest.NewCertificate("127.0.0.1", "::1", "foo.skipper.test", "bar.skipper.test"),
		},
	}.Create()
	defer p.Close()

	client := p.Client()

	get := func(url string) string {
		t.Helper()
		rsp, err := client.Get(url)
		require.NoError(t, err)
		defer rsp.Body.Close()

		b, err := io.ReadAll(rsp.Body)
		require.NoError(t, err)

		return string(b)
	}

	assert.Equal(t, "any host response", get(p.URL))
	assert.Equal(t, "foo.skipper.test response", get("https://"+net.JoinHostPort("foo.skipper.test", p.Port)))
	assert.Equal(t, "bar.skipper.test response", get("https://"+net.JoinHostPort("bar.skipper.test", p.Port)))
}
