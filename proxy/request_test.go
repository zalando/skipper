package proxy_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestRequestInvalidChunkedEncoding(t *testing.T) {
	testLog := proxy.NewTestLog()
	defer testLog.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	p := proxytest.Config{
		Routes: eskip.MustParse(fmt.Sprintf(`* -> "%s"`, backend.URL)),
	}.Create()
	defer p.Close()

	doChunkedRequest := func(t *testing.T, body string) *http.Response {
		t.Helper()

		conn, err := net.Dial("tcp", strings.TrimPrefix(p.URL, "http://"))
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("POST / HTTP/1.1\r\nHost: skipper.test\r\nTransfer-Encoding: chunked\r\n\r\n" + body))
		require.NoError(t, err)

		resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "POST"})
		require.NoError(t, err)
		resp.Body.Close()

		return resp
	}

	for _, tc := range []struct {
		body             string
		expectErrMessage string
	}{
		{
			// "!" is an invalid byte in chunk length
			body:             "!" + "\r\nabcd\r\n" + "0\r\n\r\n",
			expectErrMessage: "invalid byte in chunk length",
		},
		{
			// empty chunk length
			body:             "" + "\r\nabcd\r\n" + "0\r\n\r\n",
			expectErrMessage: "empty hex number for chunk length",
		},
		{
			// missing \r\n after first chunk
			body:             "4\r\nabcd" + "0\r\n\r\n",
			expectErrMessage: "malformed chunked encoding",
		},
	} {
		t.Run(tc.expectErrMessage, func(t *testing.T) {
			testLog.Reset()

			resp := doChunkedRequest(t, tc.body)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			err := testLog.WaitFor("failed to do backend roundtrip due to invalid request: "+tc.expectErrMessage, 100*time.Millisecond)
			if !assert.NoError(t, err) {
				t.Logf("proxy log: %s", testLog.String())
			}
		})
	}
}
