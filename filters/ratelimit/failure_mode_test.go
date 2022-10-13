package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
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
		name                string
		ratelimitFilterName string
		failClosed          bool
		wantLimit           bool
		limitStatusCode     int
	}{
		{
			name:                "test clusterRatelimit fail open",
			ratelimitFilterName: "clusterRatelimit",
			wantLimit:           false,
			limitStatusCode:     http.StatusTooManyRequests,
		},
		{
			name:                "test clusterRatelimit fail closed",
			ratelimitFilterName: "clusterRatelimit",
			failClosed:          true,
			wantLimit:           true,
			limitStatusCode:     http.StatusTooManyRequests,
		},
		{
			name:                "test clusterClientRatelimit fail open",
			ratelimitFilterName: "clusterClientRatelimit",
			wantLimit:           false,
			limitStatusCode:     http.StatusTooManyRequests,
		},
		{
			name:                "test clusterClientRatelimit fail closed",
			ratelimitFilterName: "clusterClientRatelimit",
			failClosed:          true,
			wantLimit:           true,
			limitStatusCode:     http.StatusTooManyRequests,
		},
		{
			name:                "test backendRatelimit fail open",
			ratelimitFilterName: "backendRatelimit",
			wantLimit:           false,
			limitStatusCode:     http.StatusServiceUnavailable,
		},
		{
			name:                "test backendRatelimit fail closed",
			ratelimitFilterName: "backendRatelimit",
			failClosed:          true,
			wantLimit:           true,
			limitStatusCode:     http.StatusServiceUnavailable,
		},
		{
			name:                "test clusterLeakyBucketRatelimit fail open",
			ratelimitFilterName: "clusterLeakyBucketRatelimit",
			wantLimit:           false,
			limitStatusCode:     http.StatusTooManyRequests,
		},
		{
			name:                "test clusterLeakyBucketRatelimit fail closed",
			ratelimitFilterName: "clusterLeakyBucketRatelimit",
			failClosed:          true,
			wantLimit:           true,
			limitStatusCode:     http.StatusTooManyRequests,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			fr := builtin.MakeRegistry()

			reg := ratelimit.NewSwarmRegistry(nil, &snet.RedisOptions{Addrs: []string{"127.0.0.2:6379"}}, ratelimit.Settings{})
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

			args := []interface{}{"t", 1, "1s"}
			switch tt.ratelimitFilterName {
			case filters.ClusterLeakyBucketRatelimitName:
				args = append(args, 10, 1)
			case filters.ClusterClientRatelimitName:
				args = append(args, "X-Test")
			}

			r := &eskip.Route{Filters: []*eskip.Filter{
				{Name: tt.ratelimitFilterName, Args: args}}, Backend: backend.URL}
			if tt.failClosed {
				r.Filters = append([]*eskip.Filter{{Name: fratelimit.NewFailClosed().Name()}},
					r.Filters...)
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
				},
				r)
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", proxy.URL, err)
			}
			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Fatal(err)
				return
			}
			req.Header.Set("X-Test", "foo")

			rsp, err := http.DefaultClient.Do(req)
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
