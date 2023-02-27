package diag_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrap(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
		w.(http.Flusher).Flush()
		w.Write([]byte("world"))
	}))
	defer backend.Close()

	p := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
		},
		Routes: eskip.MustParse(`
			foobarbaz: Path("/foobarbaz") -> wrapContent("foo", "baz") -> inlineContent("bar") -> <shunt>;
			chunked: Path("/chunked") -> wrapContent("foo", "baz") -> "` + backend.URL + `";
			prefix: Path("/prefix") -> wrapContent("foo", "") -> inlineContent("bar") -> <shunt>;
			suffix: Path("/suffix") -> wrapContent("", "baz") -> inlineContent("bar") -> <shunt>;
			hex: Path("/hex") -> wrapContentHex("68657861", "6d616c") -> inlineContent("deci") -> <shunt>;
		`),
	}.Create()
	defer p.Close()

	client := p.Client()

	get := func(url string) (*http.Response, string) {
		t.Helper()
		rsp, err := client.Get(url)
		require.NoError(t, err)
		defer rsp.Body.Close()

		b, err := io.ReadAll(rsp.Body)
		require.NoError(t, err)

		return rsp, string(b)
	}

	rsp, body := get(p.URL + "/foobarbaz")

	assert.Equal(t, "foobarbaz", body)
	assert.Equal(t, rsp.ContentLength, int64(9))
	assert.Equal(t, rsp.Header.Get("Content-Length"), "9")

	rsp, body = get(p.URL + "/chunked")

	assert.Equal(t, "foohelloworldbaz", body)
	assert.Equal(t, rsp.ContentLength, int64(-1))
	assert.NotContains(t, rsp.Header, "Content-Length")

	_, body = get(p.URL + "/prefix")
	assert.Equal(t, "foobar", body)

	_, body = get(p.URL + "/suffix")
	assert.Equal(t, "barbaz", body)

	_, body = get(p.URL + "/hex")
	assert.Equal(t, "hexadecimal", body)
}

func TestWrapInvalidArgs(t *testing.T) {
	registry := builtin.MakeRegistry()

	for _, def := range []string{
		`wrapContent()`,
		`wrapContent("foo")`,
		`wrapContent(1, 2)`,
		`wrapContent("foo", "bar", "baz")`,
		`wrapContentHex()`,
		`wrapContentHex("foo", "bar")`,
		`wrapContentHex("0102")`,
		`wrapContentHex("012", "ab")`, // odd length
		`wrapContentHex("01", "abc")`, // odd length
	} {
		t.Run(def, func(t *testing.T) {
			ff, err := eskip.ParseFilters(def)
			require.NoError(t, err)
			require.Len(t, ff, 1)

			f := ff[0]

			spec := registry[f.Name]
			_, err = spec.CreateFilter(f.Args)

			assert.Error(t, err)
		})
	}
}

type testCloser struct {
	io.Reader
	closed bool
}

func (c *testCloser) Close() error {
	c.closed = true
	return nil
}

func TestWrapClosesBody(t *testing.T) {
	wrap := diag.NewWrap()

	f, err := wrap.CreateFilter([]any{"foo", "bar"})
	require.NoError(t, err)

	tc := &testCloser{Reader: &bytes.Buffer{}}
	ctx := &filtertest.Context{
		FResponse: &http.Response{Body: tc},
	}

	f.Response(ctx)

	rsp := ctx.Response()
	_, err = io.Copy(io.Discard, rsp.Body)
	require.NoError(t, err)

	err = rsp.Body.Close()
	require.NoError(t, err)

	assert.True(t, tc.closed)
}
