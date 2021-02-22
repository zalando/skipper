package filters

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/accesslog"
	"github.com/zalando/skipper/filters/apiusagemonitoring"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/circuit"
	"github.com/zalando/skipper/filters/cookie"
	"github.com/zalando/skipper/filters/cors"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/filters/fadein"
	"github.com/zalando/skipper/filters/flowid"
	"github.com/zalando/skipper/filters/log"
	"github.com/zalando/skipper/filters/ratelimit"
	"github.com/zalando/skipper/filters/rfc"
	"github.com/zalando/skipper/filters/scheduler"
	"github.com/zalando/skipper/filters/sed"
	"github.com/zalando/skipper/filters/tee"
	"github.com/zalando/skipper/filters/tracing"
	"github.com/zalando/skipper/filters/xforward"
	"github.com/zalando/skipper/helpers"
	"github.com/zalando/skipper/script"
	"net"
	"net/http"
	"regexp"
	"testing"
	"time"
)

func TestFilterCreation(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Errorf("got error while finding next available port: %v", err)
		t.FailNow()
	}
	port := listener.Addr().(*net.TCPAddr).Port

	server := &http.Server{}
	defer server.Close()
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"claims_supported":["c1","c2","c3"]}`))
	})
	go server.Serve(listener)

	oAuthConfig := &auth.OAuthConfig{}
	fakeOAuthIssuerServerAddr := fmt.Sprintf("http://localhost:%d", port)
	regex := regexp.MustCompile("/[a-z]+/")
	kvPair := helpers.NewKVPair("k1", "v1")

	t.Run("BackendIsProxy()", testWithSpecFn(
		builtin.NewBackendIsProxy(),
		BackendIsProxy()))

	t.Run("ModRequestHeader()", testWithSpecFn(
		builtin.NewModRequestHeader(),
		ModRequestHeader("header", regex, "new")))

	t.Run("SetRequestHeader()", testWithSpecFn(
		builtin.NewSetRequestHeader(),
		SetRequestHeader("key", "value")))

	t.Run("AppendRequestHeader()", testWithSpecFn(
		builtin.NewAppendRequestHeader(),
		AppendRequestHeader("key", "value")))

	t.Run("DropRequestHeader()", testWithSpecFn(
		builtin.NewDropRequestHeader(),
		DropRequestHeader("key")))

	t.Run("SetResponseHeader()", testWithSpecFn(
		builtin.NewSetResponseHeader(),
		SetResponseHeader("key", "value")))

	t.Run("AppendResponseHeader()", testWithSpecFn(
		builtin.NewAppendResponseHeader(),
		AppendResponseHeader("key", "value")))

	t.Run("DropResponseHeader()", testWithSpecFn(
		builtin.NewDropResponseHeader(),
		DropResponseHeader("key")))

	t.Run("SetContextRequestHeader()", testWithSpecFn(
		builtin.NewSetContextRequestHeader(),
		SetContextRequestHeader("header", "key")))

	t.Run("AppendContextRequestHeader()", testWithSpecFn(
		builtin.NewAppendContextRequestHeader(),
		AppendContextRequestHeader("header", "key")))

	t.Run("SetContextResponseHeader()", testWithSpecFn(
		builtin.NewSetContextResponseHeader(),
		SetContextResponseHeader("header", "key")))

	t.Run("AppendContextResponseHeader()", testWithSpecFn(
		builtin.NewAppendContextResponseHeader(),
		AppendContextResponseHeader("header", "key")))

	t.Run("CopyRequestHeader()", testWithSpecFn(
		builtin.NewCopyRequestHeader(),
		CopyRequestHeader("header1", "header2")))

	t.Run("CopyResponseHeader()", testWithSpecFn(
		builtin.NewCopyResponseHeader(),
		CopyResponseHeader("header1", "header2")))

	t.Run("ModPath()", testWithSpecFn(
		builtin.NewModPath(),
		ModPath(regex, "new")))

	t.Run("SetPath()", testWithSpecFn(
		builtin.NewSetPath(),
		SetPath("new")))

	t.Run("RedirectTo()", testWithSpecFn(
		builtin.NewRedirectTo(),
		RedirectTo(301)))

	t.Run("RedirectToLocation()", testWithSpecFn(
		builtin.NewRedirectTo(),
		RedirectToLocation(302, "/foo/newBar")))

	t.Run("RedirectToLower()", testWithSpecFn(
		builtin.NewRedirectLower(),
		RedirectToLower(302)))

	t.Run("Static()", testWithSpecFn(
		builtin.NewStatic(),
		Static("/.well-known/acme-challenge/", "/tmp")))

	t.Run("StripQuery()", testWithSpecFn(
		builtin.NewStripQuery(),
		StripQuery(false)))

	t.Run("StripQuery(true)", testWithSpecFn(
		builtin.NewStripQuery(),
		StripQuery(true)))

	t.Run("PreserveHost()", testWithSpecFn(
		builtin.PreserveHost(),
		PreserveHost(false)))

	t.Run("PreserveHost(true)", testWithSpecFn(
		builtin.PreserveHost(),
		PreserveHost(true)))

	t.Run("Status()", testWithSpecFn(
		builtin.NewStatus(),
		Status(401)))

	t.Run("Compress(..., mime)", testWithSpecFn(
		builtin.NewCompress(),
		Compress("...", "image/tiff")))

	t.Run("Compress(mime)", testWithSpecFn(
		builtin.NewCompress(),
		Compress("text/html")))

	t.Run("CompressWithLevel()", testWithSpecFn(
		builtin.NewCompress(),
		CompressWithLevel(9, "image/tiff")))

	t.Run("CompressWithLevel()", testWithSpecFn(
		builtin.NewCompress(),
		CompressWithLevel(5, "...", "image/tiff")))

	t.Run("Decompress()", testWithSpecFn(
		builtin.NewDecompress(),
		Decompress()))

	t.Run("SetQuery()", testWithSpecFn(
		builtin.NewSetQuery(),
		SetQuery("key", "value")))

	t.Run("DropQuery()", testWithSpecFn(
		builtin.NewDropQuery(),
		DropQuery("key")))

	t.Run("InlineContent()", testWithSpecFn(
		builtin.NewInlineContent(),
		InlineContent("<h1>hello world</h1>")))

	t.Run("InlineContentWithMime()", testWithSpecFn(
		builtin.NewInlineContent(),
		InlineContentWithMime("<h1>hello world</h1>", "text/html")))

	t.Run("InlineContentIfStatus()", testWithSpecFn(
		builtin.NewInlineContentIfStatus(),
		InlineContentIfStatus(404, "it's not here")))

	t.Run("InlineContentIfStatusWithMime()", testWithSpecFn(
		builtin.NewInlineContentIfStatus(),
		InlineContentIfStatusWithMime(404, "<h1>it's not here</h1>", "text/html")))

	t.Run("FlowId()", testWithSpecFn(
		flowid.New(),
		FlowId(false)))

	t.Run("FlowId(reuse)", testWithSpecFn(
		flowid.New(),
		FlowId(true)))

	t.Run("Xforward()", testWithSpecFn(
		xforward.New(),
		Xforward()))

	t.Run("XforwardFirst()", testWithSpecFn(
		xforward.NewFirst(),
		XforwardFirst()))

	t.Run("RandomContent()", testWithSpecFn(
		diag.NewRandom(),
		RandomContent(3)))

	t.Run("Latency()", testWithSpecFn(
		diag.NewLatency(),
		Latency(time.Millisecond*42)))

	t.Run("Bandwidth()", testWithSpecFn(
		diag.NewBandwidth(),
		Bandwidth(42)))

	t.Run("Chunks()", testWithSpecFn(
		diag.NewChunks(),
		Chunks(42, 0)))

	t.Run("Chunks(with delay)", testWithSpecFn(
		diag.NewChunks(),
		Chunks(42, time.Millisecond*10)))

	t.Run("BackendLatency()", testWithSpecFn(
		diag.NewBackendLatency(),
		BackendLatency(time.Millisecond*42)))

	t.Run("BackendBandwidth()", testWithSpecFn(
		diag.NewBackendBandwidth(),
		BackendBandwidth(42)))

	t.Run("BackendChunks()", testWithSpecFn(
		diag.NewBackendChunks(),
		BackendChunks(42, 0)))

	t.Run("BackendChunks(with delay)", testWithSpecFn(
		diag.NewBackendChunks(),
		BackendChunks(42, time.Millisecond*10)))

	t.Run("Absorb()", testWithSpecFn(
		diag.NewAbsorb(),
		Absorb()))

	t.Run("AbsorbSilent()", testWithSpecFn(
		diag.NewAbsorbSilent(),
		AbsorbSilent()))

	t.Run("LogHeader()", testWithSpecFn(
		diag.NewLogHeader(),
		LogHeader()))

	t.Run("LogHeader(request)", testWithSpecFn(
		diag.NewLogHeader(),
		LogHeader("request")))

	t.Run("LogHeader(response)", testWithSpecFn(
		diag.NewLogHeader(),
		LogHeader("response")))

	t.Run("LogHeader(request, response)", testWithSpecFn(
		diag.NewLogHeader(),
		LogHeader("request", "response")))

	t.Run("Tee()", testWithSpecFn(
		tee.NewTee(),
		Tee("https://audit-logging.example.org")))

	t.Run("TeeWithPathMod()", testWithSpecFn(
		tee.NewTee(),
		TeeWithPathMod("https://audit-logging.example.org", regex, "/v2")))

	t.Run("Teenf()", testWithSpecFn(
		tee.NewTeeNoFollow(),
		Teenf("https://audit-logging.example.org")))

	t.Run("TeenfWithPathMod()", testWithSpecFn(
		tee.NewTeeNoFollow(),
		TeenfWithPathMod("https://audit-logging.example.org", regex, "/v2")))

	t.Run("TeeLoopback()", testWithSpecFn(
		tee.NewTeeLoopback(),
		TeeLoopback("test-A")))

	t.Run("Sed()", testWithSpecFn(
		sed.New(),
		Sed(regex, "value")))

	t.Run("SedWithMaxBuf()", testWithSpecFn(
		sed.New(),
		SedWithMaxBuf(regex, "value", 50)))

	t.Run("SedWithMaxBufAndBufHandling()", testWithSpecFn(
		sed.New(),
		SedWithMaxBufAndBufHandling(regex, "value", 50, "best-effort")))

	t.Run("SedDelim()", testWithSpecFn(
		sed.NewDelimited(),
		SedDelim(regex, "value", "\n")))

	t.Run("SedDelimWithMaxBuf()", testWithSpecFn(
		sed.NewDelimited(),
		SedDelimWithMaxBuf(regex, "value", "\n", 50)))

	t.Run("SedDelimWithMaxBufAndBufHandling()", testWithSpecFn(
		sed.NewDelimited(),
		SedDelimWithMaxBufAndBufHandling(regex, "value", "\n", 50, "abort")))

	t.Run("SedRequest()", testWithSpecFn(
		sed.NewRequest(),
		SedRequest(regex, "value")))

	t.Run("SedRequestWithMaxBuf()", testWithSpecFn(
		sed.NewRequest(),
		SedRequestWithMaxBuf(regex, "value", 50)))

	t.Run("SedRequestWithMaxBufAndBufHandling()", testWithSpecFn(
		sed.NewRequest(),
		SedRequestWithMaxBufAndBufHandling(regex, "value", 50, "best-effort")))

	t.Run("SedRequestDelim()", testWithSpecFn(
		sed.NewDelimitedRequest(),
		SedRequestDelim(regex, "value", "\n")))

	t.Run("SedRequestDelimWithMaxBuf()", testWithSpecFn(
		sed.NewDelimitedRequest(),
		SedRequestDelimWithMaxBuf(regex, "value", "\n", 50)))

	t.Run("SedRequestDelimWithMaxBufAndBufHandling()", testWithSpecFn(
		sed.NewDelimitedRequest(),
		SedRequestDelimWithMaxBufAndBufHandling(regex, "value", "\n", 50, "best-effort")))

	t.Run("BasicAuth()", testWithSpecFn(
		auth.NewBasicAuth(),
		BasicAuth("/path/to/htpasswd")))

	t.Run("BasicAuthWithRealmName()", testWithSpecFn(
		auth.NewBasicAuth(),
		BasicAuthWithRealmName("/path/to/htpasswd", "My Website")))

	t.Run("Webhook()", testWithSpecFn(
		auth.NewWebhook(0),
		Webhook("https://custom-webhook.example.org/auth")))

	t.Run("Webhook(withHeaders)", testWithSpecFn(
		auth.NewWebhook(0),
		Webhook("https://custom-webhook.example.org/auth", "X-Copy-Webhook-Header", "X-Copy-Webhook-Other-Header")))

	t.Run("OAuthTokeninfoAnyScope()", testWithSpecFn(
		auth.NewOAuthTokeninfoAnyScope("url", 0),
		OAuthTokeninfoAnyScope("s1", "s2", "s3")))

	t.Run("OAuthTokeninfoAllScope()", testWithSpecFn(
		auth.NewOAuthTokeninfoAllScope("url", 0),
		OAuthTokeninfoAllScope("s1", "s2", "s3")))

	t.Run("OAuthTokeninfoAnyKV()", testWithSpecFn(
		auth.NewOAuthTokeninfoAnyKV("url", 0),
		OAuthTokeninfoAnyKV(kvPair, kvPair, kvPair)))

	t.Run("OAuthTokeninfoAllKV()", testWithSpecFn(
		auth.NewOAuthTokeninfoAllKV("url", 0),
		OAuthTokeninfoAllKV(kvPair, kvPair, kvPair)))

	t.Run("OAuthTokenintrospectionAnyClaims()", testWithSpecFn(
		auth.NewOAuthTokenintrospectionAnyClaims(0),
		OAuthTokenintrospectionAnyClaims(fakeOAuthIssuerServerAddr, "c1", "c2", "c3")))

	t.Run("OAuthTokenintrospectionAllClaims()", testWithSpecFn(
		auth.NewOAuthTokenintrospectionAllClaims(0),
		OAuthTokenintrospectionAllClaims(fakeOAuthIssuerServerAddr, "c1", "c2", "c3")))

	t.Run("OAuthTokenintrospectionAnyKV()", testWithSpecFn(
		auth.NewOAuthTokenintrospectionAnyKV(0),
		OAuthTokenintrospectionAnyKV(fakeOAuthIssuerServerAddr, kvPair, kvPair, kvPair)))

	t.Run("OAuthTokenintrospectionAllKV()", testWithSpecFn(
		auth.NewOAuthTokenintrospectionAllKV(0),
		OAuthTokenintrospectionAllKV(fakeOAuthIssuerServerAddr, kvPair, kvPair, kvPair)))

	t.Run("SecureOAuthTokenintrospectionAnyClaims()", testWithSpecFn(
		auth.NewSecureOAuthTokenintrospectionAnyClaims(0),
		SecureOAuthTokenintrospectionAnyClaims(fakeOAuthIssuerServerAddr, "id", "secret", "c1", "c2", "c3")))

	t.Run("SecureOAuthTokenintrospectionAllClaims()", testWithSpecFn(
		auth.NewSecureOAuthTokenintrospectionAllClaims(0),
		SecureOAuthTokenintrospectionAllClaims(fakeOAuthIssuerServerAddr, "id", "secret", "c1", "c2", "c3")))

	t.Run("SecureOAuthTokenintrospectionAnyKV()", testWithSpecFn(
		auth.NewSecureOAuthTokenintrospectionAnyKV(0),
		SecureOAuthTokenintrospectionAnyKV(fakeOAuthIssuerServerAddr, "id", "secret", kvPair, kvPair, kvPair)))

	t.Run("SecureOAuthTokenintrospectionAllKV()", testWithSpecFn(
		auth.NewSecureOAuthTokenintrospectionAllKV(0),
		SecureOAuthTokenintrospectionAllKV(fakeOAuthIssuerServerAddr, "id", "secret", kvPair, kvPair, kvPair)))

	t.Run("ForwardToken()", testWithSpecFn(
		auth.NewForwardToken(),
		ForwardToken("header")))

	t.Run("ForwardToken()", testWithSpecFn(
		auth.NewForwardToken(),
		ForwardToken("header", "k1", "k2", "k3")))

	t.Run("OAuthGrant()", testWithSpecFn(
		oAuthConfig.NewGrant(),
		OAuthGrant()))

	t.Run("GrantCallback()", testWithSpecFn(
		oAuthConfig.NewGrantCallback(),
		GrantCallback()))

	t.Run("GrantClaimsQuery()", testWithSpecFn(
		oAuthConfig.NewGrantClaimsQuery(),
		GrantClaimsQuery(`/login:groups.#[=="appX-Tester"]`, `/:@_:email%"*@example.org"`)))

	t.Run("RequestCookie()", testWithSpecFn(
		cookie.NewRequestCookie(),
		RequestCookie("name", "value")))

	t.Run("OidcClaimsQuery()", testWithSpecFn(
		auth.NewOIDCQueryClaimsFilter(),
		OidcClaimsQuery(`/login:groups.#[=="appX-Tester"]`, `/:@_:email%"*@example.org"`)))

	t.Run("ResponseCookie()", testWithSpecFn(
		cookie.NewResponseCookie(),
		ResponseCookie("name", "value")))

	t.Run("ResponseCookieWithSettings()", testWithSpecFn(
		cookie.NewResponseCookie(),
		ResponseCookieWithSettings("name", "value", time.Hour, false)))

	t.Run("ResponseCookieWithSettings()", testWithSpecFn(
		cookie.NewResponseCookie(),
		ResponseCookieWithSettings("name", "value", time.Hour, true)))

	t.Run("JsCookie()", testWithSpecFn(
		cookie.NewJSCookie(),
		JsCookie("name", "value")))

	t.Run("JsCookieWithSettings()", testWithSpecFn(
		cookie.NewJSCookie(),
		JsCookieWithSettings("name", "value", time.Hour, false)))

	t.Run("JsCookieWithSettings()", testWithSpecFn(
		cookie.NewJSCookie(),
		JsCookieWithSettings("name", "value", time.Hour, true)))

	t.Run("ConsecutiveBreaker()", testWithSpecFn(
		circuit.NewConsecutiveBreaker(),
		ConsecutiveBreaker(42)))

	t.Run("ConsecutiveBreakerWithTimeout()", testWithSpecFn(
		circuit.NewConsecutiveBreaker(),
		ConsecutiveBreakerWithTimeout(42, time.Minute)))

	t.Run("ConsecutiveBreakerWithTimeoutAndHalfOpenRequests()", testWithSpecFn(
		circuit.NewConsecutiveBreaker(),
		ConsecutiveBreakerWithTimeoutAndHalfOpenRequests(42, time.Minute, 10)))

	t.Run("ConsecutiveBreakerWithTimeoutHalfOpenRequestsAndIdleTTL()", testWithSpecFn(
		circuit.NewConsecutiveBreaker(),
		ConsecutiveBreakerWithTimeoutHalfOpenRequestsAndIdleTTL(42, time.Minute, 10, time.Second*30)))

	t.Run("RateBreaker()", testWithSpecFn(
		circuit.NewRateBreaker(),
		RateBreaker(42, 1000)))

	t.Run("RateBreakerWithTimeout()", testWithSpecFn(
		circuit.NewRateBreaker(),
		RateBreakerWithTimeout(42, 1000, time.Minute)))

	t.Run("RateBreakerWithTimeoutAndHalfOpenRequests()", testWithSpecFn(
		circuit.NewRateBreaker(),
		RateBreakerWithTimeoutAndHalfOpenRequests(42, 1000, time.Minute, 10)))

	t.Run("RateBreakerWithTimeoutHalfOpenRequestsAndIdleTTL()", testWithSpecFn(
		circuit.NewRateBreaker(),
		RateBreakerWithTimeoutHalfOpenRequestsAndIdleTTL(42, 1000, time.Minute, 10, time.Second*30)))

	t.Run("DisableBreaker()", testWithSpecFn(
		circuit.NewDisableBreaker(),
		DisableBreaker()))

	t.Run("ClientRateLimit()", testWithSpecFn(
		ratelimit.NewClientRatelimit(nil),
		ClientRatelimit(3, time.Second)))

	t.Run("ClientRateLimit(with lookup headers)", testWithSpecFn(
		ratelimit.NewClientRatelimit(nil),
		ClientRatelimit(3, time.Second, "X-Foo", "X-Bar", "Authorization")))

	t.Run("Ratelimit()", testWithSpecFn(
		ratelimit.NewRatelimit(nil),
		Ratelimit(20, time.Minute)))

	t.Run("ClusterClientRatelimit()", testWithSpecFn(
		ratelimit.NewClusterClientRateLimit(nil),
		ClusterClientRatelimit("groupA", 10, time.Hour)))

	t.Run("ClusterClientRatelimit(with headers)", testWithSpecFn(
		ratelimit.NewClusterClientRateLimit(nil),
		ClusterClientRatelimit("groupA", 10, time.Hour, "X-Forwarded-For", "Authorization")))

	t.Run("ClusterRatelimit()", testWithSpecFn(
		ratelimit.NewClusterRateLimit(nil),
		ClusterRatelimit("groupA", 20, time.Hour)))

	t.Run("Lua(with path)", testWithSpecFn(
		script.NewLuaScript(),
		Lua("../../script/set_path.lua")))

	t.Run("Lua(with path and params)", testWithSpecFn(
		script.NewLuaScript(),
		Lua("../../script/set_path.lua", "myparam=foo", "other=bar")))

	t.Run("Lua(with script)", testWithSpecFn(
		script.NewLuaScript(),
		Lua("function request(c, p); print(c.request.url); end")))

	t.Run("Lua(with script and params)", testWithSpecFn(
		script.NewLuaScript(),
		Lua("function request(c, p); print(c.request.url); end", "myparam=foo", "other=bar")))

	t.Run("CorsOrigin()", testWithSpecFn(
		cors.NewOrigin(),
		CorsOrigin()))

	t.Run("CorsOrigin(with origins)", testWithSpecFn(
		cors.NewOrigin(),
		CorsOrigin("https://www.example.org", "http://localhost:9001")))

	t.Run("HeaderToQuery()", testWithSpecFn(
		builtin.NewHeaderToQuery(),
		HeaderToQuery("X-Foo-Header", "foo-query-param")))

	t.Run("QueryToHeader()", testWithSpecFn(
		builtin.NewQueryToHeader(),
		QueryToHeader("foo-query-param", "X-Foo-Header")))

	t.Run("QueryToHeaderWithFormatString()", testWithSpecFn(
		builtin.NewQueryToHeader(),
		QueryToHeaderWithFormatString("access_token", "Authorization", "Bearer %s")))

	t.Run("DisableAccessLog()", testWithSpecFn(
		accesslog.NewDisableAccessLog(),
		DisableAccessLog()))

	t.Run("DisableAccessLog(for specific statusCodes)", testWithSpecFn(
		accesslog.NewDisableAccessLog(),
		DisableAccessLog(1, 301, 40)))

	t.Run("EnableAccessLog()", testWithSpecFn(
		accesslog.NewEnableAccessLog(),
		EnableAccessLog()))

	t.Run("EnableAccessLog(for specific statusCodes)", testWithSpecFn(
		accesslog.NewEnableAccessLog(),
		EnableAccessLog(1, 301, 40)))

	t.Run("AuditLog()", testWithSpecFn(
		log.NewAuditLog(0),
		AuditLog()))

	t.Run("UnverifiedAuditLog()", testWithSpecFn(
		log.NewUnverifiedAuditLog(),
		UnverifiedAuditLog("azp")))

	t.Run("SetDynamicBackendHostFromHeader()", testWithSpecFn(
		builtin.NewSetDynamicBackendHostFromHeader(),
		SetDynamicBackendHostFromHeader("X-Forwarded-Host")))

	t.Run("SetDynamicBackendSchemeFromHeader()", testWithSpecFn(
		builtin.NewSetDynamicBackendSchemeFromHeader(),
		SetDynamicBackendSchemeFromHeader("X-Forwarded-Proto")))

	t.Run("SetDynamicBackendUrlFromHeader()", testWithSpecFn(
		builtin.NewSetDynamicBackendUrlFromHeader(),
		SetDynamicBackendUrlFromHeader("X-Custom-Url")))

	t.Run("SetDynamicBackendHost()", testWithSpecFn(
		builtin.NewSetDynamicBackendHost(),
		SetDynamicBackendHost("example.com")))

	t.Run("SetDynamicBackendScheme()", testWithSpecFn(
		builtin.NewSetDynamicBackendScheme(),
		SetDynamicBackendScheme("https")))

	t.Run("SetDynamicBackendUrl()", testWithSpecFn(
		builtin.NewSetDynamicBackendUrl(),
		SetDynamicBackendUrl("https://example.com")))

	apiUsageMonitoringSpec, err := ApiUsageMonitoring(&apiusagemonitoring.ApiConfig{ApplicationId: "some id", ApiId: "some id", PathTemplates: []string{"some template"}})
	if err != nil {
		t.Errorf("unexpected error while parsing ApiUsageMonitoring spec, %v", err)
	}

	t.Run("ApiUsageMonitoring()", testWithSpecFn(
		apiusagemonitoring.NewApiUsageMonitoring(true,
			"", "", ""), apiUsageMonitoringSpec))

	t.Run("Lifo()", testWithSpecFn(
		scheduler.NewLIFO(),
		Lifo()))

	t.Run("LifoWithCustomConcurrency()", testWithSpecFn(
		scheduler.NewLIFO(),
		LifoWithCustomConcurrency(100)))

	t.Run("LifoWithCustomConcurrencyAndQueueSize()", testWithSpecFn(
		scheduler.NewLIFO(),
		LifoWithCustomConcurrencyAndQueueSize(100, 150)))

	t.Run("LifoWithCustomConcurrencyQueueSizeAndTimeout()", testWithSpecFn(
		scheduler.NewLIFO(),
		LifoWithCustomConcurrencyQueueSizeAndTimeout(100, 150, time.Second*10)))

	t.Run("LifoGroup()", testWithSpecFn(
		scheduler.NewLIFOGroup(),
		LifoGroup("mygroup")))

	t.Run("LifoGroupWithCustomConcurrency()", testWithSpecFn(
		scheduler.NewLIFOGroup(),
		LifoGroupWithCustomConcurrency("mygroup", 100)))

	t.Run("LifoGroupWithCustomConcurrencyAndQueueSize()", testWithSpecFn(
		scheduler.NewLIFOGroup(),
		LifoGroupWithCustomConcurrencyAndQueueSize("mygroup", 100, 150)))

	t.Run("LifoGroupWithCustomConcurrencyQueueSizeAndTimeout()", testWithSpecFn(
		scheduler.NewLIFOGroup(),
		LifoGroupWithCustomConcurrencyQueueSizeAndTimeout("mygroup", 100, 150, time.Second*10)))

	t.Run("RfcPath()", testWithSpecFn(
		rfc.NewPath(),
		RfcPath()))

	t.Run("Bearerinjector()", testWithSpecFn(
		auth.NewBearerInjector(nil),
		Bearerinjector("secretName")))

	t.Run("TracingBaggageToTag()", testWithSpecFn(
		tracing.NewBaggageToTagFilter(),
		TracingBaggageToTag("baggageItemName", "tagName")))

	t.Run("StateBagToTag()", testWithSpecFn(
		tracing.NewStateBagToTag(),
		StateBagToTag("stateBagItemName", "tagName")))

	t.Run("TracingTag()", testWithSpecFn(
		tracing.NewTag(),
		TracingTag("tagName", "tagValue")))

	t.Run("OriginMarker()", testWithSpecFn(
		builtin.NewOriginMarkerSpec(),
		OriginMarker("origin", "id", time.Now())))

	t.Run("FadeIn()", testWithSpecFn(
		fadein.NewFadeIn(),
		FadeIn(time.Minute*3)))

	t.Run("FadeInWithCurve()", testWithSpecFn(
		fadein.NewFadeIn(),
		FadeInWithCurve(time.Minute*3, 1.5)))

	t.Run("EndpointCreated()", testWithSpecFn(
		fadein.NewEndpointCreated(),
		EndpointCreated("http://10.0.0.1:8080", time.Now())))
}

func TestArgumentParsing(t *testing.T) {
	// For the OAuthOidcUserInfo, OAuthOidcAnyClaims, OAuthOidcAllClaims filters we just test the argument parsing
	// as they require a working backend which is out of scope for these tests.
	testFn := func(expectedArguments int, filter *eskip.Filter) func(t *testing.T) {
		return func(t *testing.T) {
			if len(filter.Args) != expectedArguments {
				t.Errorf("Expected %d arguments, got %d for filter %s, arguments: %s", expectedArguments, len(filter.Args), filter.Name, filter.Args)
			}
		}
	}

	t.Run("OAuthOidcUserInfo()",
		testFn(6, OAuthOidcUserInfo("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture")))
	t.Run("OAuthOidcUserInfo(withAuthCodes)",
		testFn(9, OAuthOidcUserInfo("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture", "param=value", "param2=value", "param3=value")))
	t.Run("OAuthOidcUserInfo(withUpstreamHeaders)",
		testFn(9, OAuthOidcUserInfo("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture", "X-Auth-Authorization:claims.email", "X-Other:claims.other", "X-Last:claims.last")))
	t.Run("OAuthOidcUserInfo(withAuthCodesAndUpstreamHeaders)",
		testFn(8, OAuthOidcUserInfo("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture", "param=value", "X-Auth-Authorization:claims.email")))

	t.Run("OAuthOidcAnyClaims()",
		testFn(6, OAuthOidcAnyClaims("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture")))
	t.Run("OAuthOidcAnyClaims(withAuthCodes)",
		testFn(9, OAuthOidcAnyClaims("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture", "param=value", "param2=value", "param3=value")))
	t.Run("OAuthOidcAnyClaims(withUpstreamHeaders)",
		testFn(9, OAuthOidcAnyClaims("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture", "X-Auth-Authorization:claims.email", "X-Other:claims.other", "X-Last:claims.last")))
	t.Run("OAuthOidcAnyClaims(withAuthCodesAndUpstreamHeaders)",
		testFn(8, OAuthOidcAnyClaims("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture", "param=value", "X-Auth-Authorization:claims.email")))

	t.Run("OAuthOidcAllClaims()",
		testFn(6, OAuthOidcAllClaims("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture")))
	t.Run("OAuthOidcAllClaims(withAuthCodes)",
		testFn(9, OAuthOidcAllClaims("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture", "param=value", "param2=value", "param3=value")))
	t.Run("OAuthOidcAllClaims(withUpstreamHeaders)",
		testFn(9, OAuthOidcAllClaims("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture", "X-Auth-Authorization:claims.email", "X-Other:claims.other", "X-Last:claims.last")))
	t.Run("OAuthOidcAllClaims(withAuthCodesAndUpstreamHeaders)",
		testFn(8, OAuthOidcAllClaims("http://localhost:8080", "id", "secret", "url", "email profile", "name email picture", "param=value", "X-Auth-Authorization:claims.email")))
}

func testWithSpecFn(filterSpec filters.Spec, filter *eskip.Filter) func(t *testing.T) {
	return func(t *testing.T) {
		if filterSpec.Name() != filter.Name {
			t.Errorf("spec name and filter name differ, spec=%s, filter=%s", filterSpec.Name(), filter.Name)
		}
		_, err := filterSpec.CreateFilter(filter.Args)
		if err != nil {
			t.Errorf("unexpected error while parsing %s filter with args %s, %v", filter.Name, filter.Args, err)
		}
	}
}
