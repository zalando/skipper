package filters

import (
	"encoding/json"
	"github.com/zalando/skipper/eskip"
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
	"github.com/zalando/skipper/filters/rfc"
	"github.com/zalando/skipper/filters/scheduler"
	"github.com/zalando/skipper/filters/sed"
	"github.com/zalando/skipper/filters/tee"
	"github.com/zalando/skipper/filters/tracing"
	"github.com/zalando/skipper/filters/xforward"
	"github.com/zalando/skipper/helpers"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/script"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// backendIsProxy
func BackendIsProxy() *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.BackendIsProxyName,
	}
}

// modRequestHeader
func ModRequestHeader(header string, expression *regexp.Regexp, replacement string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.ModRequestHeaderName,
		Args: []interface{}{header, expression.String(), replacement},
	}
}

// setRequestHeader
func SetRequestHeader(name, value string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetRequestHeaderName,
		Args: []interface{}{name, value},
	}
}

// appendRequestHeader
func AppendRequestHeader(name, value string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.AppendRequestHeaderName,
		Args: []interface{}{name, value},
	}
}

// dropRequestHeader
func DropRequestHeader(name string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.DropRequestHeaderName,
		Args: []interface{}{name},
	}
}

// setResponseHeader
func SetResponseHeader(name, value string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetResponseHeaderName,
		Args: []interface{}{name, value},
	}
}

// appendResponseHeader
func AppendResponseHeader(name, value string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.AppendResponseHeaderName,
		Args: []interface{}{name, value},
	}
}

// dropResponseHeader
func DropResponseHeader(name string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.DropResponseHeaderName,
		Args: []interface{}{name},
	}
}

// setContextRequestHeader
func SetContextRequestHeader(headerName, key string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetContextRequestHeaderName,
		Args: []interface{}{headerName, key},
	}
}

// appendContextRequestHeader
func AppendContextRequestHeader(headerName, key string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.AppendContextRequestHeaderName,
		Args: []interface{}{headerName, key},
	}
}

// setContextResponseHeader
func SetContextResponseHeader(headerName, key string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetContextResponseHeaderName,
		Args: []interface{}{headerName, key},
	}
}

// appendContextResponseHeader
func AppendContextResponseHeader(headerName, key string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.AppendContextResponseHeaderName,
		Args: []interface{}{headerName, key},
	}
}

// copyRequestHeader
func CopyRequestHeader(sourceName, targetName string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.CopyRequestHeaderName,
		Args: []interface{}{sourceName, targetName},
	}
}

// copyResponseHeader
func CopyResponseHeader(sourceName, targetName string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.CopyResponseHeaderName,
		Args: []interface{}{sourceName, targetName},
	}
}

// modPath
func ModPath(expression *regexp.Regexp, replacement string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.ModPathName,
		Args: []interface{}{expression.String(), replacement},
	}
}

// setPath
func SetPath(replacement string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetPathName,
		Args: []interface{}{replacement},
	}
}

// redirectTo
func RedirectTo(statusCode int) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.RedirectToName,
		Args: []interface{}{float64(statusCode)},
	}
}

// redirectTo
func RedirectToLocation(statusCode int, location string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.RedirectToName,
		Args: []interface{}{float64(statusCode), location},
	}
}

// redirectToLower
func RedirectToLower(statusCode int) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.RedirectToLowerName,
		Args: []interface{}{float64(statusCode)},
	}
}

// static
func Static(requestPathToStrip, targetBasePath string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.StaticName,
		Args: []interface{}{requestPathToStrip, targetBasePath},
	}
}

// stripQuery
func StripQuery(preserveInHeader bool) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.StripQueryName,
		Args: []interface{}{strconv.FormatBool(preserveInHeader)},
	}
}

// PreserveHost
func PreserveHost(preserve bool) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.PreserveHostName,
		Args: []interface{}{strconv.FormatBool(preserve)},
	}
}

// Status
func Status(statusCode int) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.StatusName,
		Args: []interface{}{statusCode},
	}
}

// compress
func Compress(mimeTypes ...string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.CompressName,
		Args: stringSliceToArgs(mimeTypes),
	}
}

// compress
func CompressWithLevel(level int, mimeTypes ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(mimeTypes)+1)
	args = append(args, float64(level))
	for _, m := range mimeTypes {
		args = append(args, m)
	}
	return &eskip.Filter{
		Name: builtin.CompressName,
		Args: args,
	}
}

// decompress
func Decompress() *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.DecompressName,
	}
}

// setQuery
func SetQuery(key, value string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetQueryName,
		Args: []interface{}{key, value},
	}
}

// dropQuery
func DropQuery(key string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.DropQueryName,
		Args: []interface{}{key},
	}
}

// InlineContent
func InlineContent(text string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.InlineContentName,
		Args: []interface{}{text},
	}
}

// InlineContentWithMime
func InlineContentWithMime(text, mime string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.InlineContentName,
		Args: []interface{}{text, mime},
	}
}

// inlineContentIfStatus
func InlineContentIfStatus(statusCode int, text string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.InlineContentIfStatusName,
		Args: []interface{}{statusCode, text},
	}
}

// inlineContentIfStatus
func InlineContentIfStatusWithMime(statusCode int, text, mime string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.InlineContentIfStatusName,
		Args: []interface{}{statusCode, text, mime},
	}
}

// flowId
func FlowId(reuseExisting bool) *eskip.Filter {
	var args []interface{}
	if reuseExisting {
		args = []interface{}{flowid.ReuseParameterValue}
	}
	return &eskip.Filter{
		Name: flowid.Name,
		Args: args,
	}
}

// xforward
func Xforward() *eskip.Filter {
	return &eskip.Filter{
		Name: xforward.Name,
	}
}

// xforwardFirst
func XforwardFirst() *eskip.Filter {
	return &eskip.Filter{
		Name: xforward.NameFirst,
	}
}

// randomContent
func RandomContent(length int64) *eskip.Filter {
	return &eskip.Filter{
		Name: diag.RandomName,
		Args: []interface{}{float64(length)},
	}
}

// latency
func Latency(latency time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: diag.LatencyName,
		Args: []interface{}{float64(latency.Milliseconds())},
	}
}

// bandwidth
func Bandwidth(bandwidthInKbps float64) *eskip.Filter {
	return &eskip.Filter{
		Name: diag.BandwidthName,
		Args: []interface{}{bandwidthInKbps},
	}
}

// chunks
func Chunks(byteLength int, delayBetweenResponses time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: diag.ChunksName,
		Args: []interface{}{float64(byteLength), float64(delayBetweenResponses.Milliseconds())},
	}
}

// backendLatency
func BackendLatency(latency time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: diag.BackendLatencyName,
		Args: []interface{}{float64(latency.Milliseconds())},
	}
}

// backendBandwidth
func BackendBandwidth(bandwidthInKbps float64) *eskip.Filter {
	return &eskip.Filter{
		Name: diag.BackendBandwidthName,
		Args: []interface{}{bandwidthInKbps},
	}
}

// backendChunks
func BackendChunks(byteLength int, delayBetweenResponses time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: diag.BackendChunksName,
		Args: []interface{}{float64(byteLength), float64(delayBetweenResponses.Milliseconds())},
	}
}

// absorb
func Absorb() *eskip.Filter {
	return &eskip.Filter{
		Name: diag.AbsorbName,
	}
}

// absorbSilent
func AbsorbSilent() *eskip.Filter {
	return &eskip.Filter{
		Name: diag.AbsorbSilentName,
	}
}

// logHeader
func LogHeader(args ...string) *eskip.Filter {
	return &eskip.Filter{
		Name: diag.LogHeaderName,
		Args: stringSliceToArgs(args),
	}
}

// tee
func Tee(backend string) *eskip.Filter {
	return &eskip.Filter{
		Name: tee.Name,
		Args: []interface{}{backend},
	}
}

// tee
func TeeWithPathMod(backend string, pathExpression *regexp.Regexp, pathReplacement string) *eskip.Filter {
	return &eskip.Filter{
		Name: tee.Name,
		Args: []interface{}{backend, pathExpression.String(), pathReplacement},
	}
}

// teenf
func Teenf(backend string) *eskip.Filter {
	return &eskip.Filter{
		Name: tee.NoFollowName,
		Args: []interface{}{backend},
	}
}

// teenf
func TeenfWithPathMod(backend string, pathExpression *regexp.Regexp, pathReplacement string) *eskip.Filter {
	return &eskip.Filter{
		Name: tee.NoFollowName,
		Args: []interface{}{backend, pathExpression.String(), pathReplacement},
	}
}

// teeLoopback
func TeeLoopback(teeGroup string) *eskip.Filter {
	return &eskip.Filter{
		Name: tee.FilterName,
		Args: []interface{}{teeGroup},
	}
}

// sed
func Sed(pattern *regexp.Regexp, replacement string) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.Name,
		Args: []interface{}{pattern.String(), replacement},
	}
}

// sed
func SedWithMaxBuf(pattern *regexp.Regexp, replacement string, maxBuf int) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.Name,
		Args: []interface{}{pattern.String(), replacement, maxBuf},
	}
}

// sed
func SedWithMaxBufAndBufHandling(pattern *regexp.Regexp, replacement string, maxBuf int, maxBufHandling string) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.Name,
		Args: []interface{}{pattern.String(), replacement, maxBuf, maxBufHandling},
	}
}

// sedDelim
func SedDelim(pattern *regexp.Regexp, replacement, delimiterString string) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.NameDelimit,
		Args: []interface{}{pattern.String(), replacement, delimiterString},
	}
}

// sedDelim
func SedDelimWithMaxBuf(pattern *regexp.Regexp, replacement, delimiterString string, maxBuf int) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.NameDelimit,
		Args: []interface{}{pattern.String(), replacement, delimiterString, maxBuf},
	}
}

// sedDelim
func SedDelimWithMaxBufAndBufHandling(pattern *regexp.Regexp, replacement, delimiterString string, maxBuf int, maxBufHandling string) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.NameDelimit,
		Args: []interface{}{pattern.String(), replacement, delimiterString, maxBuf, maxBufHandling},
	}
}

// sedRequest
func SedRequest(pattern *regexp.Regexp, replacement string) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.NameRequest,
		Args: []interface{}{pattern.String(), replacement},
	}
}

// sedRequest
func SedRequestWithMaxBuf(pattern *regexp.Regexp, replacement string, maxBuf int) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.NameRequest,
		Args: []interface{}{pattern.String(), replacement, maxBuf},
	}
}

// sedRequest
func SedRequestWithMaxBufAndBufHandling(pattern *regexp.Regexp, replacement string, maxBuf int, maxBufHandling string) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.NameRequest,
		Args: []interface{}{pattern.String(), replacement, maxBuf, maxBufHandling},
	}
}

// sedRequestDelim
func SedRequestDelim(pattern *regexp.Regexp, replacement, delimiterString string) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.NameRequestDelimit,
		Args: []interface{}{pattern.String(), replacement, delimiterString},
	}
}

// sedRequestDelim
func SedRequestDelimWithMaxBuf(pattern *regexp.Regexp, replacement, delimiterString string, maxBuf int) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.NameRequestDelimit,
		Args: []interface{}{pattern.String(), replacement, delimiterString, maxBuf},
	}
}

// sedRequestDelim
func SedRequestDelimWithMaxBufAndBufHandling(pattern *regexp.Regexp, replacement, delimiterString string, maxBuf int, maxBufHandling string) *eskip.Filter {
	return &eskip.Filter{
		Name: sed.NameRequestDelimit,
		Args: []interface{}{pattern.String(), replacement, delimiterString, maxBuf, maxBufHandling},
	}
}

// basicAuth
func BasicAuth(pathToHtpasswd string) *eskip.Filter {
	return &eskip.Filter{
		Name: auth.Name,
		Args: []interface{}{pathToHtpasswd},
	}
}

// basicAuth
func BasicAuthWithRealmName(pathToHtpasswd string, realmName string) *eskip.Filter {
	return &eskip.Filter{
		Name: auth.Name,
		Args: []interface{}{pathToHtpasswd, realmName},
	}
}

// webhook
func Webhook(webhook string, headersFromWebhookToRequest ...string) *eskip.Filter {
	var args []interface{}
	if len(headersFromWebhookToRequest) > 0 {
		args = []interface{}{webhook, strings.Join(headersFromWebhookToRequest, ",")}
	} else {
		args = []interface{}{webhook}
	}
	return &eskip.Filter{
		Name: auth.WebhookName,
		Args: args,
	}
}

// oauthTokeninfoAnyScope
func OAuthTokeninfoAnyScope(scopes ...string) *eskip.Filter {
	return &eskip.Filter{
		Name: auth.OAuthTokeninfoAnyScopeName,
		Args: stringSliceToArgs(scopes),
	}
}

// oauthTokeninfoAllScope
func OAuthTokeninfoAllScope(scopes ...string) *eskip.Filter {
	return &eskip.Filter{
		Name: auth.OAuthTokeninfoAllScopeName,
		Args: stringSliceToArgs(scopes),
	}
}

// oauthTokeninfoAnyKV
func OAuthTokeninfoAnyKV(pairs ...helpers.KVPair) *eskip.Filter {
	return &eskip.Filter{
		Name: auth.OAuthTokeninfoAnyKVName,
		Args: helpers.KVPairToArgs(pairs),
	}
}

// oauthTokeninfoAllKV
func OAuthTokeninfoAllKV(pairs ...helpers.KVPair) *eskip.Filter {
	return &eskip.Filter{
		Name: auth.OAuthTokeninfoAllKVName,
		Args: helpers.KVPairToArgs(pairs),
	}
}

// oauthTokenintrospectionAnyClaims
func OAuthTokenintrospectionAnyClaims(issuerURL string, claims ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(claims)+1)
	args = append(args, issuerURL)
	for _, c := range claims {
		args = append(args, c)
	}
	return &eskip.Filter{
		Name: auth.OAuthTokenintrospectionAnyClaimsName,
		Args: args,
	}
}

// oauthTokenintrospectionAllClaims
func OAuthTokenintrospectionAllClaims(issuerURL string, claims ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(claims)+1)
	args = append(args, issuerURL)
	for _, c := range claims {
		args = append(args, c)
	}
	return &eskip.Filter{
		Name: auth.OAuthTokenintrospectionAllClaimsName,
		Args: args,
	}
}

// oauthTokenintrospectionAnyKV
func OAuthTokenintrospectionAnyKV(issuerURL string, pairs ...helpers.KVPair) *eskip.Filter {
	args := make([]interface{}, 0, (len(pairs)*2)+1)
	args = append(args, issuerURL)
	for _, p := range pairs {
		args = append(args, p.Key, p.Value)
	}
	return &eskip.Filter{
		Name: auth.OAuthTokenintrospectionAnyKVName,
		Args: args,
	}
}

// oauthTokenintrospectionAllKV
func OAuthTokenintrospectionAllKV(issuerURL string, pairs ...helpers.KVPair) *eskip.Filter {
	args := make([]interface{}, 0, (len(pairs)*2)+1)
	args = append(args, issuerURL)
	for _, p := range pairs {
		args = append(args, p.Key, p.Value)
	}
	return &eskip.Filter{
		Name: auth.OAuthTokenintrospectionAllKVName,
		Args: args,
	}
}

// secureOauthTokenintrospectionAnyClaims
func SecureOAuthTokenintrospectionAnyClaims(issuerURL, clientId, clientSecret string, claims ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(claims)+3)
	args = append(args, issuerURL, clientId, clientSecret)
	for _, c := range claims {
		args = append(args, c)
	}
	return &eskip.Filter{
		Name: auth.SecureOAuthTokenintrospectionAnyClaimsName,
		Args: args,
	}
}

// secureOauthTokenintrospectionAllClaims
func SecureOAuthTokenintrospectionAllClaims(issuerURL, clientId, clientSecret string, claims ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(claims)+3)
	args = append(args, issuerURL, clientId, clientSecret)
	for _, c := range claims {
		args = append(args, c)
	}
	return &eskip.Filter{
		Name: auth.SecureOAuthTokenintrospectionAllClaimsName,
		Args: args,
	}
}

// secureOauthTokenintrospectionAnyKV
func SecureOAuthTokenintrospectionAnyKV(issuerURL, clientId, clientSecret string, pairs ...helpers.KVPair) *eskip.Filter {
	args := make([]interface{}, 0, (len(pairs)*2)+3)
	args = append(args, issuerURL, clientId, clientSecret)
	for _, p := range pairs {
		args = append(args, p.Key, p.Value)
	}
	return &eskip.Filter{
		Name: auth.SecureOAuthTokenintrospectionAnyKVName,
		Args: args,
	}
}

// secureOauthTokenintrospectionAllKV
func SecureOAuthTokenintrospectionAllKV(issuerURL, clientId, clientSecret string, pairs ...helpers.KVPair) *eskip.Filter {
	args := make([]interface{}, 0, (len(pairs)*2)+3)
	args = append(args, issuerURL, clientId, clientSecret)
	for _, p := range pairs {
		args = append(args, p.Key, p.Value)
	}
	return &eskip.Filter{
		Name: auth.SecureOAuthTokenintrospectionAllKVName,
		Args: args,
	}
}

// forwardToken
func ForwardToken(headerName string, whiteListedJsonKeys ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(whiteListedJsonKeys)+1)
	args = append(args, headerName)
	for _, k := range whiteListedJsonKeys {
		args = append(args, k)
	}
	return &eskip.Filter{
		Name: auth.ForwardTokenName,
		Args: args,
	}
}

// oauthGrant
func OAuthGrant() *eskip.Filter {
	return &eskip.Filter{
		Name: auth.OAuthGrantName,
	}
}

// grantCallback
func GrantCallback() *eskip.Filter {
	return &eskip.Filter{
		Name: auth.GrantCallbackName,
	}
}

// grantClaimsQuery
func GrantClaimsQuery(pathQueries ...string) *eskip.Filter {
	return &eskip.Filter{
		Name: auth.GrantClaimsQueryName,
		Args: stringSliceToArgs(pathQueries),
	}
}

// oauthOidcUserInfo
func OAuthOidcUserInfo(openIdConnectProviderURL, clientId, clientSecret, callbackURL, scopes, claims string, optionalParams ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(optionalParams)+6)
	args = append(args, openIdConnectProviderURL, clientId, clientSecret, callbackURL, scopes, claims)
	for _, p := range optionalParams {
		args = append(args, p)
	}
	return &eskip.Filter{
		Name: auth.OidcUserInfoName,
		Args: args,
	}
}

// oauthOidcAnyClaims
func OAuthOidcAnyClaims(openIdConnectProviderURL, clientId, clientSecret, callbackURL, scopes, claims string, optionalParams ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(optionalParams)+6)
	args = append(args, openIdConnectProviderURL, clientId, clientSecret, callbackURL, scopes, claims)
	for _, p := range optionalParams {
		args = append(args, p)
	}
	return &eskip.Filter{
		Name: auth.OidcAnyClaimsName,
		Args: args,
	}
}

// oauthOidcAllClaims
func OAuthOidcAllClaims(openIdConnectProviderURL, clientId, clientSecret, callbackURL, scopes, claims string, optionalParams ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(optionalParams)+6)
	args = append(args, openIdConnectProviderURL, clientId, clientSecret, callbackURL, scopes, claims)
	for _, p := range optionalParams {
		args = append(args, p)
	}
	return &eskip.Filter{
		Name: auth.OidcAllClaimsName,
		Args: args,
	}
}

// requestCookie
func RequestCookie(name, value string) *eskip.Filter {
	return &eskip.Filter{
		Name: cookie.RequestCookieFilterName,
		Args: []interface{}{name, value},
	}
}

// oidcClaimsQuery
func OidcClaimsQuery(pathQueries ...string) *eskip.Filter {
	return &eskip.Filter{
		Name: auth.OidcClaimsQueryName,
		Args: stringSliceToArgs(pathQueries),
	}
}

// ResponseCookie
func ResponseCookie(name, value string) *eskip.Filter {
	return &eskip.Filter{
		Name: cookie.ResponseCookieFilterName,
		Args: []interface{}{name, value},
	}
}

// ResponseCookieWithSettings
// fixme: tests
func ResponseCookieWithSettings(name, value string, ttl time.Duration, changeOnly bool) *eskip.Filter {
	coParam := ""
	if changeOnly {
		coParam = cookie.ChangeOnlyArg
	}
	return &eskip.Filter{
		Name: cookie.ResponseCookieFilterName,
		Args: []interface{}{name, value, ttl.Seconds(), coParam},
	}
}

// jsCookie
func JsCookie(name, value string) *eskip.Filter {
	return &eskip.Filter{
		Name: cookie.ResponseJSCookieFilterName,
		Args: []interface{}{name, value},
	}
}

// jsCookie
func JsCookieWithSettings(name, value string, ttl time.Duration, changeOnly bool) *eskip.Filter {
	coParam := ""
	if changeOnly {
		coParam = cookie.ChangeOnlyArg
	}
	return &eskip.Filter{
		Name: cookie.ResponseJSCookieFilterName,
		Args: []interface{}{name, value, ttl.Seconds(), coParam},
	}
}

// consecutiveBreaker
func ConsecutiveBreaker(consecutiveFailures int) *eskip.Filter {
	return &eskip.Filter{
		Name: circuit.ConsecutiveBreakerName,
		Args: []interface{}{consecutiveFailures},
	}
}

// consecutiveBreaker
func ConsecutiveBreakerWithTimeout(consecutiveFailures int, timeout time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: circuit.ConsecutiveBreakerName,
		Args: []interface{}{consecutiveFailures, timeout.String()},
	}
}

// consecutiveBreaker
func ConsecutiveBreakerWithTimeoutAndHalfOpenRequests(consecutiveFailures int, timeout time.Duration, halfOpenRequests int) *eskip.Filter {
	return &eskip.Filter{
		Name: circuit.ConsecutiveBreakerName,
		Args: []interface{}{consecutiveFailures, timeout.String(), halfOpenRequests},
	}
}

// consecutiveBreaker
func ConsecutiveBreakerWithTimeoutHalfOpenRequestsAndIdleTTL(consecutiveFailures int, timeout time.Duration, halfOpenRequests int, idleTTL time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: circuit.ConsecutiveBreakerName,
		Args: []interface{}{consecutiveFailures, timeout.String(), halfOpenRequests, idleTTL.String()},
	}
}

// rateBreaker
func RateBreaker(consecutiveFailures, slidingWindow int) *eskip.Filter {
	return &eskip.Filter{
		Name: circuit.RateBreakerName,
		Args: []interface{}{consecutiveFailures, slidingWindow},
	}
}

// rateBreaker
func RateBreakerWithTimeout(consecutiveFailures, slidingWindow int, timeout time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: circuit.RateBreakerName,
		Args: []interface{}{consecutiveFailures, slidingWindow, timeout.String()},
	}
}

// rateBreaker
func RateBreakerWithTimeoutAndHalfOpenRequests(consecutiveFailures, slidingWindow int, timeout time.Duration, halfOpenRequests int) *eskip.Filter {
	return &eskip.Filter{
		Name: circuit.RateBreakerName,
		Args: []interface{}{consecutiveFailures, slidingWindow, timeout.String(), halfOpenRequests},
	}
}

// rateBreaker
func RateBreakerWithTimeoutHalfOpenRequestsAndIdleTTL(consecutiveFailures, slidingWindow int, timeout time.Duration, halfOpenRequests int, idleTTL time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: circuit.RateBreakerName,
		Args: []interface{}{consecutiveFailures, slidingWindow, timeout.String(), halfOpenRequests, idleTTL.String()},
	}
}

// disableBreaker
func DisableBreaker() *eskip.Filter {
	return &eskip.Filter{
		Name: circuit.DisableBreakerName,
	}
}

// clientRatelimit
func ClientRatelimit(allowedRequests int, period time.Duration, lookupHeaders ...string) *eskip.Filter {
	args := []interface{}{allowedRequests, period.String()}
	if len(lookupHeaders) > 0 {
		args = append(args, strings.Join(lookupHeaders, ","))
	}
	return &eskip.Filter{
		Name: ratelimit.ClientRatelimitName,
		Args: args,
	}
}

// ratelimit
func Ratelimit(allowedRequests int, period time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: ratelimit.ServiceRatelimitName,
		Args: []interface{}{allowedRequests, period.String()},
	}
}

// clusterClientRatelimit
func ClusterClientRatelimit(group string, allowedRequests int, period time.Duration, lookupHeaders ...string) *eskip.Filter {
	args := []interface{}{group, allowedRequests, period.String()}
	if len(lookupHeaders) > 0 {
		args = append(args, strings.Join(lookupHeaders, ","))
	}
	return &eskip.Filter{
		Name: ratelimit.ClusterClientRatelimitName,
		Args: args,
	}
}

// clusterRatelimit
func ClusterRatelimit(group string, allowedRequests int, period time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: ratelimit.ClusterServiceRatelimitName,
		Args: []interface{}{group, allowedRequests, period.String()},
	}
}

// lua
func Lua(pathOrScript string, params ...string) *eskip.Filter {
	args := make([]interface{}, 0, len(params)+1)
	args = append(args, pathOrScript)
	for _, p := range params {
		args = append(args, p)
	}
	return &eskip.Filter{
		Name: script.LuaFilterName,
		Args: args,
	}
}

// corsOrigin
func CorsOrigin(acceptableOriginParameters ...string) *eskip.Filter {
	return &eskip.Filter{
		Name: cors.Name,
		Args: stringSliceToArgs(acceptableOriginParameters),
	}
}

// headerToQuery
func HeaderToQuery(headerName, queryParamName string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.HeaderToQueryName,
		Args: []interface{}{headerName, queryParamName},
	}
}

// queryToHeader
func QueryToHeader(queryParamName, headerName string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.QueryToHeaderName,
		Args: []interface{}{queryParamName, headerName},
	}
}

// queryToHeader
func QueryToHeaderWithFormatString(queryParamName, headerName, formatString string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.QueryToHeaderName,
		Args: []interface{}{queryParamName, headerName, formatString},
	}
}

// disableAccessLog
func DisableAccessLog(forResponseCodes ...int) *eskip.Filter {
	args := make([]interface{}, 0, len(forResponseCodes))
	for _, code := range forResponseCodes {
		args = append(args, code)
	}
	return &eskip.Filter{
		Name: accesslog.DisableAccessLogName,
		Args: args,
	}
}

// enableAccessLog
func EnableAccessLog(forResponseCodes ...int) *eskip.Filter {
	args := make([]interface{}, 0, len(forResponseCodes))
	for _, code := range forResponseCodes {
		args = append(args, code)
	}
	return &eskip.Filter{
		Name: accesslog.EnableAccessLogName,
		Args: args,
	}
}

// auditLog
func AuditLog() *eskip.Filter {
	return &eskip.Filter{
		Name: log.AuditLogName,
	}
}

// unverifiedAuditLog
func UnverifiedAuditLog(authorizationTokenKeys ...string) *eskip.Filter {
	return &eskip.Filter{
		Name: log.UnverifiedAuditLogName,
		Args: stringSliceToArgs(authorizationTokenKeys),
	}
}

// setDynamicBackendHostFromHeader
func SetDynamicBackendHostFromHeader(headerName string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetDynamicBackendHostFromHeader,
		Args: []interface{}{headerName},
	}
}

// setDynamicBackendSchemeFromHeader
func SetDynamicBackendSchemeFromHeader(headerName string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetDynamicBackendSchemeFromHeader,
		Args: []interface{}{headerName},
	}
}

// setDynamicBackendUrlFromHeader
func SetDynamicBackendUrlFromHeader(headerName string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetDynamicBackendUrlFromHeader,
		Args: []interface{}{headerName},
	}
}

// setDynamicBackendHost
func SetDynamicBackendHost(host string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetDynamicBackendHost,
		Args: []interface{}{host},
	}
}

// setDynamicBackendScheme
func SetDynamicBackendScheme(scheme string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetDynamicBackendScheme,
		Args: []interface{}{scheme},
	}
}

// setDynamicBackendUrl
func SetDynamicBackendUrl(url string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.SetDynamicBackendUrl,
		Args: []interface{}{url},
	}
}

// apiUsageMonitoring
func ApiUsageMonitoring(apiConfigs ...*apiusagemonitoring.ApiConfig) (*eskip.Filter, error) {
	args := make([]interface{}, 0, len(apiConfigs))
	for _, config := range apiConfigs {
		marshalledConfig, err := json.Marshal(config)
		if err != nil {
			return nil, err
		}
		args = append(args, string(marshalledConfig))
	}
	return &eskip.Filter{
		Name: apiusagemonitoring.Name,
		Args: args,
	}, nil
}

// lifo
func Lifo() *eskip.Filter {
	return &eskip.Filter{
		Name: scheduler.LIFOName,
	}
}

// lifo
func LifoWithCustomConcurrency(maxConcurrency int) *eskip.Filter {
	return &eskip.Filter{
		Name: scheduler.LIFOName,
		Args: []interface{}{maxConcurrency},
	}
}

// lifo
func LifoWithCustomConcurrencyAndQueueSize(maxConcurrency, maxQueueSize int) *eskip.Filter {
	return &eskip.Filter{
		Name: scheduler.LIFOName,
		Args: []interface{}{maxConcurrency, maxQueueSize},
	}
}

// lifo
func LifoWithCustomConcurrencyQueueSizeAndTimeout(maxConcurrency, maxQueueSize int, timeout time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: scheduler.LIFOName,
		Args: []interface{}{maxConcurrency, maxQueueSize, timeout.String()},
	}
}

// lifoGroup
func LifoGroup(groupName string) *eskip.Filter {
	return &eskip.Filter{
		Name: scheduler.LIFOGroupName,
		Args: []interface{}{groupName},
	}
}

// lifoGroup
func LifoGroupWithCustomConcurrency(groupName string, maxConcurrency int) *eskip.Filter {
	return &eskip.Filter{
		Name: scheduler.LIFOGroupName,
		Args: []interface{}{groupName, maxConcurrency},
	}
}

// lifoGroup
func LifoGroupWithCustomConcurrencyAndQueueSize(groupName string, maxConcurrency, maxQueueSize int) *eskip.Filter {
	return &eskip.Filter{
		Name: scheduler.LIFOGroupName,
		Args: []interface{}{groupName, maxConcurrency, maxQueueSize},
	}
}

// lifoGroup
func LifoGroupWithCustomConcurrencyQueueSizeAndTimeout(groupName string, maxConcurrency, maxQueueSize int, timeout time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: scheduler.LIFOGroupName,
		Args: []interface{}{groupName, maxConcurrency, maxQueueSize, timeout.String()},
	}
}

// rfcPath
func RfcPath() *eskip.Filter {
	return &eskip.Filter{
		Name: rfc.Name,
	}
}

// bearerinjector
func Bearerinjector(secretName string) *eskip.Filter {
	return &eskip.Filter{
		Name: auth.BearerInjectorName,
		Args: []interface{}{secretName},
	}
}

// tracingBaggageToTag
func TracingBaggageToTag(baggageItemName, tagName string) *eskip.Filter {
	return &eskip.Filter{
		Name: tracing.BaggageToTagFilterName,
		Args: []interface{}{baggageItemName, tagName},
	}
}

// stateBagToTag
func StateBagToTag(stateBagItemName, tagName string) *eskip.Filter {
	return &eskip.Filter{
		Name: tracing.StateBagToTagFilterName,
		Args: []interface{}{stateBagItemName, tagName},
	}
}

// tracingTag
func TracingTag(tagName, tagValue string) *eskip.Filter {
	return &eskip.Filter{
		Name: tracing.Name,
		Args: []interface{}{tagName, tagValue},
	}
}

// originMarker
func OriginMarker(origin, id string, creation time.Time) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.OriginMarkerName,
		Args: []interface{}{origin, id, creation},
	}
}

// fadeIn
func FadeIn(duration time.Duration) *eskip.Filter {
	return &eskip.Filter{
		Name: fadein.FadeInName,
		Args: []interface{}{duration},
	}
}

// fadeIn
func FadeInWithCurve(duration time.Duration, curve float64) *eskip.Filter {
	return &eskip.Filter{
		Name: fadein.FadeInName,
		Args: []interface{}{duration, curve},
	}
}

// endpointCreated
func EndpointCreated(address string, createdAt time.Time) *eskip.Filter {
	return &eskip.Filter{
		Name: fadein.EndpointCreatedName,
		Args: []interface{}{address, float64(createdAt.Unix())},
	}
}

func stringSliceToArgs(strings []string) []interface{} {
	args := make([]interface{}, 0, len(strings))
	for _, s := range strings {
		args = append(args, s)
	}
	return args
}
