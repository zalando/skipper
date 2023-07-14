package proxy_test

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/proxy/proxytest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultResponse(t *testing.T) {
	p := proxytest.Config{
		Routes: eskip.MustParse(`* -> <shunt>`),
	}.Create()
	defer p.Close()

	rsp, body, err := p.Client().GetBody(p.URL)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, rsp.StatusCode)
	assert.Len(t, body, 0)
	assert.Equal(t, "0", rsp.Header.Get("Content-Length"))
}
