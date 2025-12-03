package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	fratelimit "github.com/zalando/skipper/filters/ratelimit"
	snet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/routing"
)

func TestFailureMode(t *testing.T) {
	for _, tt := range []struct {
		name            string
		filters         string
		wantLimit       bool
		limitStatusCode int
	}{
		{
			name:            "test clusterRatelimit fail open",
			filters:         `clusterRatelimit("t", 1, "1s")`,
			wantLimit:       false,
			limitStatusCode: http.StatusTooManyRequests,
		},
		{
			name:            "test clusterRatelimit fail closed",
			filters:         `ratelimitFailClosed() -> clusterRatelimit("t", 1, "1s")`,
			wantLimit:       true,
			limitStatusCode: http.StatusTooManyRequests,
		},
		{
			name:            "test clusterClientRatelimit fail open",
			filters:         `clusterClientRatelimit("t", 1, "1s", "X-Test")`,
			wantLimit:       false,
			limitStatusCode: http.StatusTooManyRequests,
		},
		{
			name:            "test clusterClientRatelimit fail closed",
			filters:         `ratelimitFailClosed() -> clusterClientRatelimit("t", 1, "1s", "X-Test")`,
			wantLimit:       true,
			limitStatusCode: http.StatusTooManyRequests,
		},
		{
			name:            "test backendRatelimit fail open",
			filters:         `backendRatelimit("t", 1, "1s")`,
			wantLimit:       false,
			limitStatusCode: http.StatusServiceUnavailable,
		},
		{
			name:            "test backendRatelimit fail closed",
			filters:         `ratelimitFailClosed() -> backendRatelimit("t", 1, "1s")`,
			wantLimit:       true,
			limitStatusCode: http.StatusServiceUnavailable,
		},
		{
			name:            "test clusterLeakyBucketRatelimit fail open",
			filters:         `clusterLeakyBucketRatelimit("t", 1, "1s")`,
			wantLimit:       false,
			limitStatusCode: http.StatusTooManyRequests,
		},
		{
			name:            "test clusterLeakyBucketRatelimit fail closed",
			filters:         `ratelimitFailClosed() -> clusterLeakyBucketRatelimit("t", 1, "1s", 10, 1)`,
			wantLimit:       true,
			limitStatusCode: http.StatusTooManyRequests,
		},
		{
			name:            "test ratelimitFailClosed applies when placed after ratelimit filter",
			filters:         `clusterRatelimit("t", 1, "1s") -> ratelimitFailClosed()`,
			wantLimit:       true,
			limitStatusCode: http.StatusTooManyRequests,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fr := builtin.MakeRegistry()

			reg := ratelimit.NewSwarmRegistry(nil, &snet.RedisOptions{Addrs: []string{"fails.test:6379"}}, ratelimit.Settings{})
			defer reg.Close()

			provider := fratelimit.NewRatelimitProvider(reg)
			fr.Register(fratelimit.NewClusterRateLimit(provider))
			fr.Register(fratelimit.NewClusterClientRateLimit(provider))
			fr.Register(fratelimit.NewClusterLeakyBucketRatelimit(reg))
			fr.Register(fratelimit.NewBackendRatelimit())
			fr.Register(fratelimit.NewFailClosed())

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			r := &eskip.Route{
				Filters: eskip.MustParseFilters(tt.filters),
				Backend: backend.URL,
			}

			proxy := proxytest.WithParamsAndRoutingOptions(
				fr,
				proxy.Params{
					RateLimiters: reg,
				},
				routing.Options{
					PostProcessors: []routing.PostProcessor{
						fratelimit.NewFailClosedPostProcessor(),
					},
				}, r)
			defer proxy.Close()

			req, err := http.NewRequest("GET", proxy.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-Test", "foo")

			rsp, err := proxy.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer rsp.Body.Close()

			limited := rsp.StatusCode == tt.limitStatusCode
			if tt.wantLimit && !limited {
				t.Errorf("Failed to get limited response, got: limited=%v status=%d", limited, rsp.StatusCode)
			} else if !tt.wantLimit && limited {
				t.Errorf("Failed to get allowed response, got: limited=%v", limited)
			}
		})
	}
}
