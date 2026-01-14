## How to develop a Filter

A filter is part of a route and can change arbitrary http data in the
`http.Request` and `http.Response` path of a proxy.

The filter example shows a non trivial diff of a filter
implementation, that implements an authnz webhook. It shows global
settings passed via flags, user documentation, developer documentation
for library users, the filter implementation and some test
cases. Tests should test the actual filter implementation in a proxy
setup.

### How to pass options to your filter

Set a `default` and a `Usage` string as const.  Add a var to hold the
value and put the flag to the category, that makes the most sense.

If a filter, predicate or dataclient need `Options` passed from flags,
then you should register the filter in `skipper.go`, the main library
entrypoint. In case you do not need options from flags, use
`MakeRegistry()` in `./filters/builtin/builtin.go` to register your
filter.


```diff
diff --git a/cmd/skipper/main.go b/cmd/skipper/main.go
index 28f18f9..4530b85 100644
--- a/cmd/skipper/main.go
+++ b/cmd/skipper/main.go
@@ -59,9 +59,10 @@ const (
 	defaultOAuthTokeninfoTimeout          = 2 * time.Second
 	defaultOAuthTokenintrospectionTimeout = 2 * time.Second
+	defaultWebhookTimeout                 = 2 * time.Second

 	// generic:
 	addressUsage                         = "network address that skipper should listen on"
@@ -141,6 +142,8 @@ const (
 	oauth2TokeninfoURLUsage              = "sets the default tokeninfo URL to query information about an incoming OAuth2 token in oauth2Tokeninfo filters"
 	oauth2TokeninfoTimeoutUsage          = "sets the default tokeninfo request timeout duration to 2000ms"
 	oauth2TokenintrospectionTimeoutUsage = "sets the default tokenintrospection request timeout duration to 2000ms"
+	webhookTimeoutUsage                  = "sets the webhook request timeout duration, defaults to 2s"
+
 	// connections, timeouts:
 	idleConnsPerHostUsage           = "maximum idle connections per backend host"
 	closeIdleConnsPeriodUsage       = "period of closing all idle connections in seconds or as a duration string. Not closing when less than 0"
@@ -243,13 +246,14 @@ var (
 	oauth2TokeninfoURL              string
 	oauth2TokeninfoTimeout          time.Duration
 	oauth2TokenintrospectionTimeout time.Duration
+	webhookTimeout                  time.Duration

 	// connections, timeouts:
 	idleConnsPerHost           int
@@ -351,13 +355,14 @@ func init() {
 	flag.DurationVar(&oauth2TokeninfoTimeout, "oauth2-tokeninfo-timeout", defaultOAuthTokeninfoTimeout, oauth2TokeninfoTimeoutUsage)
 	flag.DurationVar(&oauth2TokenintrospectionTimeout, "oauth2-tokenintrospect-timeout", defaultOAuthTokenintrospectionTimeout, oauth2TokenintrospectionTimeoutUsage)
+	flag.DurationVar(&webhookTimeout, "webhook-timeout", defaultWebhookTimeout, webhookTimeoutUsage)

 	// connections, timeouts:
 	flag.IntVar(&idleConnsPerHost, "idle-conns-num", proxy.DefaultIdleConnsPerHost, idleConnsPerHostUsage)
@@ -536,13 +541,14 @@ func main() {
 		OAuthTokeninfoURL:              oauth2TokeninfoURL,
 		OAuthTokeninfoTimeout:          oauth2TokeninfoTimeout,
 		OAuthTokenintrospectionTimeout: oauth2TokenintrospectionTimeout,
+		WebhookTimeout:                 webhookTimeout,

 		// connections, timeouts:
 		IdleConnectionsPerHost:     idleConnsPerHost,

diff --git a/skipper.go b/skipper.go
index 10d5769..da46fe0 100644
--- a/skipper.go
+++ b/skipper.go
@@ -443,6 +443,9 @@ type Options struct {
 	// OAuthTokenintrospectionTimeout sets timeout duration while calling oauth tokenintrospection service
 	OAuthTokenintrospectionTimeout time.Duration

+	// WebhookTimeout sets timeout duration while calling a custom webhook auth service
+	WebhookTimeout time.Duration
+
 	// MaxAuditBody sets the maximum read size of the body read by the audit log filter
 	MaxAuditBody int
 }
@@ -677,7 +680,8 @@ func Run(o Options) error {
 		auth.NewOAuthTokenintrospectionAnyClaims(o.OAuthTokenintrospectionTimeout),
 		auth.NewOAuthTokenintrospectionAllClaims(o.OAuthTokenintrospectionTimeout),
 		auth.NewOAuthTokenintrospectionAnyKV(o.OAuthTokenintrospectionTimeout),
-		auth.NewOAuthTokenintrospectionAllKV(o.OAuthTokenintrospectionTimeout))
+		auth.NewOAuthTokenintrospectionAllKV(o.OAuthTokenintrospectionTimeout),
+		auth.NewWebhook(o.WebhookTimeout))

 	// create a filter registry with the available filter specs registered,
 	// and register the custom filters
```

### User documentation

Documentation for users should be done in `docs/`.

````diff
diff --git a/docs/filters.md b/docs/filters.md
index d3bb872..a877062 100644
--- a/docs/filters.md
+++ b/docs/filters.md
@@ -382,6 +382,24 @@ basicAuth("/path/to/htpasswd")
 basicAuth("/path/to/htpasswd", "My Website")
 ```

+## webhook
+
+The `webhook` filter makes it possible to have your own authentication and
+authorization endpoint as a filter.
+
+Headers from the incoming request will be copied into the request that
+is being done to the webhook endpoint. Responses from the webhook with
+status code less than 300 will be authorized, rest unauthorized.
+
+Examples:
+
+```
+webhook("https://custom-webhook.example.org/auth")
+```
+
+The webhook timeout has a default of 2 seconds and can be globally
+changed, if skipper is started with `-webhook-timeout=2s` flag.
+
 ## oauthTokeninfoAnyScope

 If skipper is started with `-oauth2-tokeninfo-url` flag, you can use
````

### Add godoc

Godoc is meant for developers using skipper as library, use `doc.go`
of the package to document generic functionality, usage and library
usage.

```diff
diff --git a/filters/auth/doc.go b/filters/auth/doc.go
index 696d3fd..1d6e3a8 100644
--- a/filters/auth/doc.go
+++ b/filters/auth/doc.go
@@ -318,5 +318,12 @@ filter after the auth filter.
     a: Path("/only-allowed-audit-log") -> oauthTokeninfoAnyScope("bar-w") -> auditLog() -> "https://internal.example.org/";
     b: Path("/all-access-requests-audit-log") -> auditLog() -> oauthTokeninfoAnyScope("foo-r") -> "https://internal.example.org/";

+Webhook - webhook() filter
+
+The filter webhook allows you to have a custom authentication and
+authorization endpoint for a route.
+
+    a: Path("/only-allowed-by-webhook") -> webhook("https://custom-webhook.example.org/auth") -> "https://protected-backend.example.org/";
+
 */
 package auth
```

### Filter implementation

A filter can modify the incoming `http.Request` before calling the
backend and the outgoing `http.Response` from the backend to the client.

A filter consists of at least two types a `spec` and a `filter`.
Spec consists of everything that is needed and known before a user
will instantiate a filter.

A spec will be created in the bootstrap procedure of a skipper
process. A spec has to satisfy the `Spec` interface `Name() string` and
`CreateFilter([]interface{}) (filters.Filter, error)`.

The actual filter implementation has to satisfy the `Filter`
interface `Request(filters.FilterContext)` and `Response(filters.FilterContext)`.

```diff
diff --git a/filters/auth/webhook.go b/filters/auth/webhook.go
new file mode 100644
index 0000000..f0632a6
--- /dev/null
+++ b/filters/auth/webhook.go
@@ -0,0 +1,84 @@
+package auth
+
+import (
+	"net/http"
+	"time"
+
+	"github.com/zalando/skipper/filters"
+)
+
+const (
+	WebhookName = "webhook"
+)
+
+type (
+	webhookSpec struct {
+		Timeout time.Duration
+	}
+	webhookFilter struct {
+		authClient *authClient
+	}
+)
+
+// NewWebhook creates a new auth filter specification
+// to validate authorization for requests.
+func NewWebhook(d time.Duration) filters.Spec {
+	return &webhookSpec{Timeout: d}
+}
+
+func (*webhookSpec) Name() string {
+	return WebhookName
+}
+
+// CreateFilter creates an auth filter. The first argument is an URL
+// string.
+//
+//     s.CreateFilter("https://my-auth-service.example.org/auth")
+//
+func (ws *webhookSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
+	if l := len(args); l == 0 || l > 2 {
+		return nil, filters.ErrInvalidFilterParameters
+	}
+
+	s, ok := args[0].(string)
+	if !ok {
+		return nil, filters.ErrInvalidFilterParameters
+	}
+
+	ac, err := newAuthClient(s, ws.Timeout)
+	if err != nil {
+		return nil, filters.ErrInvalidFilterParameters
+	}
+
+	return &webhookFilter{authClient: ac}, nil
+}
+
+func copyHeader(to, from http.Header) {
+	for k, v := range from {
+		to[http.CanonicalHeaderKey(k)] = v
+	}
+}
+
+func (f *webhookFilter) Request(ctx filters.FilterContext) {
+	statusCode, err := f.authClient.getWebhook(ctx.Request())
+	if err != nil {
+		unauthorized(ctx, WebhookName, authServiceAccess, f.authClient.url.Hostname())
+	}
+	// redirects, auth errors, webhook errors
+	if statusCode >= 300 {
+		unauthorized(ctx, WebhookName, invalidAccess, f.authClient.url.Hostname())
+	}
+	authorized(ctx, WebhookName)
+}
+
+func (*webhookFilter) Response(filters.FilterContext) {}
```

### Writing tests

Skipper uses normal table driven Go tests without frameworks.

This example filter test creates a backend, an auth service to be
called by our filter, and a filter configured by our table driven test.

In general we use real backends with dynamic port allocations. We call
these and inspect the `http.Response` to check, if we get expected
results for invalid and valid data.

Skipper has some helpers to create the test proxy in the `proxytest`
package. Backends can be created with `httptest.NewServer` as in the
example below.


```diff
diff --git a/filters/auth/webhook_test.go b/filters/auth/webhook_test.go
new file mode 100644
index 0000000..d43c4ea
--- /dev/null
+++ b/filters/auth/webhook_test.go
@@ -0,0 +1,128 @@
+package auth
+
+import (
+	"fmt"
+	"io"
+	"net/http"
+	"net/http/httptest"
+	"net/url"
+	"testing"
+	"time"
+
+	"github.com/zalando/skipper/eskip"
+	"github.com/zalando/skipper/filters"
+	"github.com/zalando/skipper/proxy/proxytest"
+)
+
+func TestWebhook(t *testing.T) {
+	for _, ti := range []struct {
+		msg        string
+		token      string
+		expected   int
+		authorized bool
+		timeout    bool
+	}{{
+		msg:        "invalid-token-should-be-unauthorized",
+		token:      "invalid-token",
+		expected:   http.StatusUnauthorized,
+		authorized: false,
+	}, {
+		msg:        "valid-token-should-be-authorized",
+		token:      testToken,
+		expected:   http.StatusOK,
+		authorized: true,
+	}, {
+		msg:        "webhook-timeout-should-be-unauthorized",
+		token:      testToken,
+		expected:   http.StatusUnauthorized,
+		authorized: false,
+		timeout:    true,
+	}} {
+		t.Run(ti.msg, func(t *testing.T) {
+			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
+				w.WriteHeader(http.StatusOK)
+				io.WriteString(w, "Hello from backend")
+				return
+			}))
+			defer backend.Close()
+
+			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+				if ti.timeout {
+					time.Sleep(time.Second + time.Millisecond)
+				}
+
+				if r.Method != "GET" {
+					w.WriteHeader(489)
+					io.WriteString(w, "FAIL - not a GET request")
+					return
+				}
+
+				tok := r.Header.Get(authHeaderName)
+				tok = tok[len(authHeaderPrefix):len(tok)]
+				switch tok {
+				case testToken:
+					w.WriteHeader(200)
+					fmt.Fprintln(w, "OK - Got token: "+tok)
+					return
+				}
+				w.WriteHeader(402)                            //http.StatusUnauthorized)
+				fmt.Fprintln(w, "Unauthorized - Got token: ") //+tok)
+			}))
+			defer authServer.Close()
+
+			spec := NewWebhook(time.Second)
+
+			args := []interface{}{
+				"http://" + authServer.Listener.Addr().String(),
+			}
+			f, err := spec.CreateFilter(args)
+			if err != nil {
+				t.Errorf("error in creating filter for %s: %v", ti.msg, err)
+				return
+			}
+
+			f2 := f.(*webhookFilter)
+			defer f2.Close()
+
+			fr := make(filters.Registry)
+			fr.Register(spec)
+			r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}
+
+			proxy := proxytest.New(fr, r)
+			defer proxy.Close()
+
+			reqURL, err := url.Parse(proxy.URL)
+			if err != nil {
+				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
+				return
+			}
+
+			req, err := http.NewRequest("GET", reqURL.String(), nil)
+			if err != nil {
+				t.Errorf("failed to create request %v", err)
+				return
+			}
+			req.Header.Set(authHeaderName, authHeaderPrefix+ti.token)
+
+			rsp, err := http.DefaultClient.Do(req)
+			if err != nil {
+				t.Errorf("failed to get response: %v", err)
+				return
+			}
+			defer rsp.Body.Close()
+
+			buf := make([]byte, 128)
+			var n int
+			if n, err = rsp.Body.Read(buf); err != nil && err != io.EOF {
+				t.Errorf("Could not read response body: %v", err)
+				return
+			}
+
+			t.Logf("%d %d", rsp.StatusCode, ti.expected)
+			if rsp.StatusCode != ti.expected {
+				t.Errorf("unexpected status code: %v != %v %d %s", rsp.StatusCode, ti.expected, n, buf)
+				return
+			}
+		})
+	}
+}
```

### Using a debugger
Skipper supports plugins and to offer this support it uses the [`plugin`](https://golang.org/pkg/plugin/)
library. Due to [Go compiler issue #23733](https://github.com/golang/go/issues/23733), a
debugger cannot be used. This issue will be fixed in Go 1.12 but until then the only workaround is to remove
references to the `plugin` library. The following patch can be used for debugging.

```diff
diff --git a/plugins.go b/plugins.go
index 837b6cf..aa69f09 100644
--- a/plugins.go
+++ b/plugins.go
@@ -1,5 +1,6 @@
 package skipper

+/*
 import (
 	"fmt"
 	"os"
@@ -13,8 +14,13 @@ import (
 	"github.com/zalando/skipper/filters"
 	"github.com/zalando/skipper/routing"
 )
+*/

 func (o *Options) findAndLoadPlugins() error {
+	return nil
+}
+
+/*
 	found := make(map[string]string)
 	done := make(map[string][]string)

@@ -366,3 +372,4 @@ func readPluginConfig(plugin string) (conf []string, err error) {
 	}
 	return conf, nil
 }
+*/

```

The patch can be applied with the `git apply $PATCH_FILE` command. Please do not commit the
modified `plugins.go` along with your changes.
