package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokeninfoCache(t *testing.T) {
	var authRequests int32

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&authRequests, 1)
		fmt.Fprint(w, `{"uid": "foo", "scope":["uid", "bar"], "expires_in": 600}`)
	}))
	defer authServer.Close()

	o := TokeninfoOptions{
		URL:       authServer.URL + "/oauth2/tokeninfo",
		CacheSize: 1,
		CacheTTL:  300 * time.Second, // less than "expires_in"
	}
	c, err := o.newTokeninfoClient()
	require.NoError(t, err)

	now := time.Now()
	c.(*tokeninfoCache).now = func() time.Time {
		return now
	}

	ctx := &filtertest.Context{FRequest: &http.Request{}}

	const token = "whatever"

	info, err := c.getTokeninfo(token, ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(1), authRequests)
	assert.Equal(t, map[string]any{"expires_in": float64(600), "uid": "foo", "scope": []any{"uid", "bar"}}, info)

	// "sleep" fractional number of seconds
	const delay = float64(5.7)
	now = now.Add(time.Duration(delay * float64(time.Second)))

	cachedInfo, err := c.getTokeninfo(token, ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(1), authRequests)
	assert.Equal(t, map[string]any{"expires_in": float64(595), "uid": "foo", "scope": []any{"uid", "bar"}}, cachedInfo)
}

type mockTokeninfoClient map[string]map[string]any

func (c mockTokeninfoClient) getTokeninfo(token string, _ filters.FilterContext) (map[string]any, error) {
	return c[token], nil
}

var infoSink map[string]any

func BenchmarkTokeninfoCache(b *testing.B) {
	ctx := &filtertest.Context{FRequest: &http.Request{}}

	for _, bi := range []struct {
		name      string
		cacheSize int
		tokens    map[string]map[string]any
	}{
		{
			name:      "one token, no eviction",
			cacheSize: 1,
			tokens: map[string]map[string]any{
				"first": {"first": "foo", "expires_in": float64(600)},
			},
		},
		{
			name:      "two tokens, no eviction",
			cacheSize: 2,
			tokens: map[string]map[string]any{
				"first":  {"uid": "first", "expires_in": float64(600)},
				"second": {"uid": "second", "expires_in": float64(600)},
			},
		},
		{
			name:      "four tokens, with eviction",
			cacheSize: 2,
			tokens: map[string]map[string]any{
				"first":  {"uid": "first", "expires_in": float64(600)},
				"second": {"uid": "second", "expires_in": float64(600)},
				"third":  {"uid": "third", "expires_in": float64(600)},
				"fourth": {"uid": "fourth", "expires_in": float64(600)},
			},
		},
	} {
		b.Run(bi.name, func(b *testing.B) {
			mc := mockTokeninfoClient(bi.tokens)
			c := newTokeninfoCache(mc, bi.cacheSize, time.Hour)

			var tokens []string
			for token := range bi.tokens {
				tokens = append(tokens, token)

				_, err := c.getTokeninfo(token, ctx)
				require.NoError(b, err)
			}

			b.ReportAllocs()
			b.ResetTimer()

			info := infoSink
			for i := 0; i < b.N; i++ {
				token := tokens[i%len(tokens)]

				info, _ = c.getTokeninfo(token, ctx)
			}
			infoSink = info
		})
	}
}
