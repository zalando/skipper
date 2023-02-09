package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tokeninfoClientFunc func(string, filters.FilterContext) (map[string]any, error)

func (f tokeninfoClientFunc) getTokeninfo(token string, ctx filters.FilterContext) (map[string]any, error) {
	return f(token, ctx)
}

type testTokeninfoToken string

func newTestTokeninfoToken(issuedAt time.Time) testTokeninfoToken {
	return testTokeninfoToken(issuedAt.Format(time.RFC3339Nano))
}

func (t testTokeninfoToken) issuedAt() time.Time {
	at, _ := time.Parse(time.RFC3339Nano, string(t))
	return at
}

func (t testTokeninfoToken) String() string {
	return string(t)
}

type testClock struct {
	mu sync.Mutex
	time.Time
}

func (c *testClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Time = c.Time.Add(d)
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Time
}

func TestTokeninfoCache(t *testing.T) {
	const (
		TokenTTLSeconds = 600
		CacheTTL        = 300 * time.Second // less than TokenTTLSeconds
	)

	start, err := time.Parse(time.RFC3339, "2022-11-10T00:36:41+01:00")
	require.NoError(t, err)

	clock := testClock{Time: start}

	var authRequests int32
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&authRequests, 1)

		token := testTokeninfoToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))

		elapsed := clock.now().Sub(token.issuedAt())
		expiresIn := (TokenTTLSeconds*time.Second - elapsed).Truncate(time.Second).Seconds()

		fmt.Fprintf(w, `{"uid": "%s", "scope":["foo", "bar"], "expires_in": %.0f}`, token, expiresIn)
	}))
	defer authServer.Close()

	o := TokeninfoOptions{
		URL:       authServer.URL + "/oauth2/tokeninfo",
		CacheSize: 1,
		CacheTTL:  CacheTTL,
	}
	c, err := o.newTokeninfoClient()
	require.NoError(t, err)

	defer c.(*tokeninfoCache).client.(*authClient).Close()
	c.(*tokeninfoCache).now = clock.now

	ctx := &filtertest.Context{FRequest: &http.Request{}}

	token := newTestTokeninfoToken(clock.now()).String()

	// First request
	info, err := c.getTokeninfo(token, ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(1), authRequests)
	assert.Equal(t, info["uid"], token)
	assert.Equal(t, info["expires_in"], float64(600), "expected TokenTTLSeconds")

	// Second request after "sleeping" fractional number of seconds
	const delay = float64(5.7)
	clock.add(time.Duration(delay * float64(time.Second)))

	info, err = c.getTokeninfo(token, ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(1), authRequests, "expected no request to auth sever")
	assert.Equal(t, info["uid"], token)
	assert.Equal(t, info["expires_in"], float64(595), "expected TokenTTLSeconds - truncate(delay)")

	// Third request after "sleeping" longer than cache TTL
	clock.add(CacheTTL)

	info, err = c.getTokeninfo(token, ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(2), authRequests, "expected new request to auth sever")
	assert.Equal(t, info["uid"], token)
	assert.Equal(t, info["expires_in"], float64(294), "expected truncate(TokenTTLSeconds - CacheTTL - delay)")

	// Fourth request with a new token evicts cached value
	token = newTestTokeninfoToken(clock.now()).String()

	info, err = c.getTokeninfo(token, ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(3), authRequests, "expected new request to auth sever")
	assert.Equal(t, info["uid"], token)
	assert.Equal(t, info["expires_in"], float64(600), "expected TokenTTLSeconds")
}

// Tests race between reading and writing cache for the same token
func TestTokeninfoCacheUpdateRace(t *testing.T) {
	var (
		authRequests int32

		firstReceived = make(chan struct{})
		firstContinue = make(chan struct{})
	)

	mc := tokeninfoClientFunc(func(token string, _ filters.FilterContext) (map[string]any, error) {
		requestNumber := atomic.AddInt32(&authRequests, 1)
		if requestNumber == 1 {
			close(firstReceived)
			<-firstContinue
		}
		return map[string]any{"requestNumber": requestNumber, "uid": token, "expires_in": float64(600)}, nil
	})

	c := newTokeninfoCache(mc, 1, time.Hour)

	const token = "atoken"

	type result struct {
		info map[string]any
		err  error
	}

	// perform first request
	firstResult := make(chan result)
	go func() {
		info, err := c.getTokeninfo(token, &filtertest.Context{FRequest: &http.Request{}})
		firstResult <- result{info, err}
	}()

	// wait until first request is blocked inside tokenInfo client
	<-firstReceived

	// perform second request
	info, err := c.getTokeninfo(token, &filtertest.Context{FRequest: &http.Request{}})
	require.NoError(t, err)
	assert.Equal(t, int32(2), info["requestNumber"], "expected response to the second auth request")

	// unblock first request
	close(firstContinue)

	// check first request result
	r := <-firstResult
	info, err = r.info, r.err
	require.NoError(t, err)
	assert.Equal(t, int32(1), info["requestNumber"], "expected response to the first auth request")

	// perform third request and check cached value
	info, err = c.getTokeninfo(token, &filtertest.Context{FRequest: &http.Request{}})
	require.NoError(t, err)
	assert.Equal(t, int32(1), info["requestNumber"], "expected to cache response to the first auth request")
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
			tokens:      100,
			cacheSize:   100,
			parallelism: 10_000,
		},
		{
			tokens:    4,
			cacheSize: 2,
		},
		{
			tokens:    100,
			cacheSize: 10,
		},
	} {
		name := fmt.Sprintf("tokens=%d,cacheSize=%d,p=%d", bi.tokens, bi.cacheSize, bi.parallelism)

		b.Run(name, func(b *testing.B) {
			tokenValues := make(map[string]map[string]any, bi.tokens)
			mc := tokeninfoClientFunc(func(token string, _ filters.FilterContext) (map[string]any, error) {
				time.Sleep(20 * time.Millisecond)
				return tokenValues[token], nil
			})

			c := newTokeninfoCache(mc, bi.cacheSize, time.Hour)

			var tokens []string
			for i := 0; i < bi.tokens; i++ {
				token := fmt.Sprintf("token-%0700d", i)

				tokenValues[token] = map[string]any{"uid": token, "expires_in": float64(600)}
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
