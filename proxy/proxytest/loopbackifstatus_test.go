package proxytest

import (
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"io"
	"net/url"
	"testing"
)

func TestLoopbackWithResponse(t *testing.T) {
	// create registry
	registry := builtin.MakeRegistry()

	// create and register the filter specification
	spec := &loopbackIfStatusSpec{}
	registry.Register(spec)

	routes := eskip.MustParse(`
INTERNAL_REDIRECT: Path("/internal-redirect/") -> status(418) -> loopbackIfStatus(418, "/tea-pot") -> <shunt>;
NO_RESULTS: Path("/tea-pot") -> status(200) -> inlineContent("I'm a teapot, not a search engine", "text/plain'") -> <shunt>;
`)

	p := New(registry, routes...)
	defer p.Close()

	client := p.Client()

	get := func(url *url.URL) (int, string) {
		t.Helper()
		rsp, err := client.Get(url.String())
		require.NoError(t, err)
		defer rsp.Body.Close()

		b, err := io.ReadAll(rsp.Body)
		require.NoError(t, err)

		return rsp.StatusCode, string(b)
	}

	for _, ti := range []struct {
		msg            string
		input          *url.URL
		expectedStatus int
	}{
		{
			msg:            "request to internal redirect",
			input:          getUrl("/internal-redirect/"),
			expectedStatus: 200,
		},
		{
			msg:            "request to tea-pot",
			input:          getUrl("/tea-pot"),
			expectedStatus: 200,
		},
		{
			msg:            "request to non-existing path",
			input:          getUrl("/non-existing"),
			expectedStatus: 404,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			status, _ := get(ti.input)
			require.Equal(t, ti.expectedStatus, status, "unexpected status code for %s", ti.msg)
		})
	}
}

func getUrl(path string) *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   "example.com",
		Path:   path,
	}
}
