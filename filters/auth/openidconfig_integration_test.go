package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestOpenIDConfigSharedByAuthFilters(t *testing.T) {
	cache := NewOpenIDConfigCache()
	defer cache.Close()
	jwks := setupJWKSServer(t, privateKey)
	defer jwks.Close()
	var requests atomic.Int32
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprintf(w, `{"jwks_uri":%q,"introspection_endpoint":%q,"claims_supported":["sub"]}`, jwks.URL, jwks.URL+"/introspection")
	}))
	defer issuer.Close()
	if _, err := NewJwtValidationWithOptions(TokenintrospectionOptions{OpenIDConfigCache: cache}).CreateFilter([]any{issuer.URL}); err != nil {
		t.Fatal(err)
	}
	issuer.Close()
	if _, err := NewJwtValidationWithOptions(TokenintrospectionOptions{}).CreateFilter([]any{issuer.URL}); err == nil {
		t.Fatal("default JWT discovery reused an enabled caller's cache entry")
	}

	for _, tt := range []struct {
		create func(time.Duration) filters.Spec
		secure bool
		kv     bool
	}{
		{create: NewOAuthTokenintrospectionAnyClaims},
		{create: NewOAuthTokenintrospectionAllClaims},
		{create: NewOAuthTokenintrospectionAnyKV, kv: true},
		{create: NewOAuthTokenintrospectionAllKV, kv: true},
		{create: NewSecureOAuthTokenintrospectionAnyClaims, secure: true},
		{create: NewSecureOAuthTokenintrospectionAllClaims, secure: true},
		{create: NewSecureOAuthTokenintrospectionAnyKV, secure: true, kv: true},
		{create: NewSecureOAuthTokenintrospectionAllKV, secure: true, kv: true},
	} {
		spec := TokenintrospectionWithOptions(tt.create, TokenintrospectionOptions{Timeout: time.Second, OpenIDConfigCache: cache})
		t.Run(spec.Name(), func(t *testing.T) {
			args := []any{issuer.URL}
			if tt.secure {
				args = append(args, spec.Name(), "client-secret")
			}
			args = append(args, "sub")
			if tt.kv {
				args = append(args, "subject")
			}
			f, err := spec.CreateFilter(args)
			if err != nil {
				t.Fatalf("shared discovery unavailable: %v", err)
			}
			user := f.(*tokenintrospectFilter).authClient.url.User
			if tt.secure {
				if user == nil || user.Username() != spec.Name() {
					t.Fatal("secure client credentials changed")
				}
				if secret, _ := user.Password(); secret != "client-secret" {
					t.Fatal("secure client secret changed")
				}
			} else if user != nil {
				t.Fatal("insecure client inherited credentials")
			}
			if _, err := tt.create(time.Second).CreateFilter(args); err == nil {
				t.Fatal("default introspection discovery reused an enabled caller's cache entry")
			}
			if !tt.kv {
				args[len(args)-1] = "unsupported"
				if _, err := spec.CreateFilter(args); err == nil {
					t.Fatal("cached discovery bypassed supported-claim validation")
				}
			}
		})
	}
	if requests.Load() != 1 {
		t.Fatalf("auth filters did not share one discovery: %d", requests.Load())
	}
}

type discoveryRoutingMetrics struct {
	metrics.Metrics
	applied chan struct{}
}

func (m *discoveryRoutingMetrics) MeasureSince(key string, _ time.Time) {
	if key == "routes.update_latency" {
		m.applied <- struct{}{}
	}
}

func TestOpenIDConfigRoutingRebuildOutage(t *testing.T) {
	cache := NewOpenIDConfigCache()
	defer cache.Close()
	var unavailable atomic.Bool
	var discoveries atomic.Int32
	jwks := setupJWKSServer(t, privateKey)
	defer jwks.Close()
	var version atomic.Int32
	version.Store(1)
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		discoveries.Add(1)
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, `{"jwks_uri":%q}`, fmt.Sprintf("%s/keys/%d", jwks.URL, version.Load()))
	}))
	defer issuer.Close()

	const protectedRoutes = 5
	defs := make([]*eskip.Route, protectedRoutes)
	for i := range defs {
		defs[i] = &eskip.Route{
			Id:          fmt.Sprintf("protected%d", i),
			Predicates:  []*eskip.Predicate{{Name: "Path", Args: []any{fmt.Sprintf("/protected/%d", i)}}},
			Filters:     []*eskip.Filter{{Name: filters.JwtValidationName, Args: []any{issuer.URL}}},
			BackendType: eskip.ShuntBackend,
		}
	}
	client := testdataclient.New(defs)
	defer client.Close()
	registry := make(filters.Registry)
	registry.Register(NewJwtValidationWithOptions(TokenintrospectionOptions{Timeout: time.Second, OpenIDConfigCache: cache}))
	m := &discoveryRoutingMetrics{Metrics: metrics.Void, applied: make(chan struct{}, 1)}
	router := routing.New(routing.Options{
		FilterRegistry: registry,
		DataClients:    []routing.DataClient{client},
		PollTimeout:    time.Millisecond,
		Metrics:        m,
	})
	defer router.Close()
	waitApplied := func() {
		t.Helper()
		select {
		case <-m.applied:
		case <-time.After(5 * time.Second):
			t.Fatal("routing table was not applied")
		}
	}
	lookup := func(path string) *routing.Route {
		route, _ := router.Route(httptest.NewRequest(http.MethodGet, path, nil))
		return route
	}
	update := func(n int) {
		t.Helper()
		client.Update([]*eskip.Route{{
			Id:          fmt.Sprintf("unrelated%d", n),
			Predicates:  []*eskip.Predicate{{Name: "Path", Args: []any{fmt.Sprintf("/unrelated/%d", n)}}},
			BackendType: eskip.ShuntBackend,
		}}, nil)
		waitApplied()
		if lookup(fmt.Sprintf("/unrelated/%d", n)) == nil {
			t.Fatal("unrelated route update was not published")
		}
	}
	waitApplied()
	oldRoute := lookup("/protected/0")
	if oldRoute == nil || discoveries.Swap(0) != 1 {
		t.Fatal("initial routes did not share discovery")
	}
	unavailable.Store(true)
	update(1)
	for i := range protectedRoutes {
		if lookup(fmt.Sprintf("/protected/%d", i)) == nil {
			t.Fatalf("warm discovery lost protected route %d during outage", i)
		}
	}
	if discoveries.Load() != 0 {
		t.Fatal("warm routing rebuild contacted unavailable issuer")
	}

	warmRoute := lookup("/protected/0")
	validToken := createTokenWithKey(t, privateKey, jwt.MapClaims{"sub": "subject", "exp": time.Now().Add(time.Hour).Unix()})
	expiredToken := createTokenWithKey(t, privateKey, jwt.MapClaims{"sub": "subject", "exp": time.Now().Add(-time.Hour).Unix()})
	for _, tt := range []struct {
		name   string
		token  string
		denied bool
	}{
		{name: "valid", token: validToken},
		{name: "expired", token: expiredToken, denied: true},
		{name: "invalid", token: "invalid", denied: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected/0", nil)
			req.Header.Set(authHeaderName, authHeaderPrefix+tt.token)
			ctx := &filtertest.Context{FRequest: req, FStateBag: make(map[string]any)}
			warmRoute.Filters[0].Request(ctx)
			if ctx.Served() != tt.denied {
				t.Fatalf("JWT authorization denied=%t, want %t", ctx.Served(), tt.denied)
			}
		})
	}

	cache.clear()
	update(2)
	if lookup("/protected/0") != nil || discoveries.Swap(0) != protectedRoutes {
		t.Fatal("cold discovery failure did not invalidate protected routes or was cached")
	}
	unavailable.Store(false)
	version.Store(2)
	update(3)
	newRoute := lookup("/protected/0")
	if newRoute == nil || discoveries.Load() != 1 {
		t.Fatal("recovered routing table did not share discovery")
	}
	oldURI := oldRoute.Filters[0].Filter.(*jwtValidationFilter).jwksUri
	newURI := newRoute.Filters[0].Filter.(*jwtValidationFilter).jwksUri
	if oldURI != jwks.URL+"/keys/1" || newURI != jwks.URL+"/keys/2" {
		t.Fatalf("rediscovery did not preserve old filter URI and update new filter URI: %q, %q", oldURI, newURI)
	}
}
