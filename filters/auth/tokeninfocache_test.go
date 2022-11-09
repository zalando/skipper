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

var infoSink atomic.Value

func BenchmarkTokeninfoCache(b *testing.B) {
	for _, bi := range []struct {
		tokens      int
		cacheSize   int
		parallelism int
	}{
		{
			tokens:    1,
			cacheSize: 1,
		},
		{
			tokens:    2,
			cacheSize: 2,
		},
		{
			tokens:    100,
			cacheSize: 100,
		},
		{
			tokens:    4,
			cacheSize: 2,
		},
		{
			tokens:    100,
			cacheSize: 10,
		},
		{
			tokens:      100,
			cacheSize:   100,
			parallelism: 10_000,
		},
	} {
		name := fmt.Sprintf("tokens=%d,cacheSize=%d,p=%d", bi.tokens, bi.cacheSize, bi.parallelism)
		b.Run(name, func(b *testing.B) {
			mc := mockTokeninfoClient(make(map[string]map[string]any, bi.tokens))
			c := newTokeninfoCache(mc, bi.cacheSize, time.Hour)

			var tokens []string
			for i := 0; i < bi.tokens; i++ {
				token := fmt.Sprintf("token-%0700d", i)

				mc[token] = map[string]any{"uid": token, "expires_in": float64(600)}
				tokens = append(tokens, token)

				_, err := c.getTokeninfo(token, &filtertest.Context{FRequest: &http.Request{}})
				require.NoError(b, err)
			}

			if bi.parallelism != 0 {
				b.SetParallelism(bi.parallelism)
			}

			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				ctx := &filtertest.Context{FRequest: &http.Request{}}
				var info map[string]any

				for i := 0; pb.Next(); i++ {
					token := tokens[i%len(tokens)]

					info, _ = c.getTokeninfo(token, ctx)
				}
				infoSink.Store(info)
			})
		})
	}
}
