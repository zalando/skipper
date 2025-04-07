package logging

import (
	"fmt"
	"maps"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/sirupsen/logrus"

	flowidFilter "github.com/zalando/skipper/filters/flowid"
	logFilter "github.com/zalando/skipper/filters/log"
)

const (
	dateFormat      = "02/Jan/2006:15:04:05 -0700"
	commonLogFormat = `%s - %s [%s] "%s %s %s" %d %d`
	// format:
	// remote_host - - [date] "method uri protocol" status response_size "referer" "user_agent"
	combinedLogFormat = commonLogFormat + ` "%s" "%s"`
	// We add the duration in ms, a requested host and a flow id and audit log
	accessLogFormat = combinedLogFormat + " %d %s %s %s\n"

	// KeyMaskedQueryParams represents the key used to store and retrieve masked query parameters
	// from the additional data.
	KeyMaskedQueryParams = "maskedQueryParams"
)

type accessLogFormatter struct {
	format string
}

// AccessEntry is the access log entry.
type AccessEntry struct {

	// The client request.
	Request *http.Request

	// The status code of the response.
	StatusCode int

	// The size of the response in bytes.
	ResponseSize int64

	// The time spent processing request.
	Duration time.Duration

	// The time that the request was received.
	RequestTime time.Time

	// The id of the authenticated user
	AuthUser string
}

// TODO: create individual instances from the access log and
// delegate the ownership from the package level to the user
// code.
var (
	accessLog  *logrus.Logger
	stripQuery bool
)

// strip port from addresses with hostname, ipv4 or ipv6
func stripPort(address string) string {
	if h, _, err := net.SplitHostPort(address); err == nil {
		return h
	}

	return address
}

// The remote host of the client. When the 'X-Forwarded-For'
// header is set, then its value is used as is.
func remoteHost(r *http.Request) string {
	ff := r.Header.Get("X-Forwarded-For")
	if ff != "" {
		return ff
	}
	return stripPort(r.RemoteAddr)
}

func omitWhitespace(h string) string {
	if h != "" {
		return h
	}
	return "-"
}

func (f *accessLogFormatter) Format(e *logrus.Entry) ([]byte, error) {
	keys := []string{
		"host", "auth-user", "timestamp", "method", "uri", "proto",
		"status", "response-size", "referer", "user-agent",
		"duration", "requested-host", "flow-id", "audit"}

	values := make([]interface{}, len(keys))
	for i, key := range keys {
		if s, ok := e.Data[key].(string); ok {
			values[i] = omitWhitespace(s)
		} else {
			values[i] = e.Data[key]
		}
	}

	return []byte(fmt.Sprintf(f.format, values...)), nil
}

func stripQueryString(u string) string {
	if i := strings.IndexRune(u, '?'); i < 0 {
		return u
	} else {
		return u[:i]
	}
}

// maskQueryParams masks (i.e., hashing) specific query parameters in the provided request's URI.
// Returns the obfuscated URI.
func maskQueryParams(req *http.Request, maskedQueryParams map[string]bool) string {
	strippedURI := stripQueryString(req.RequestURI)

	params := req.URL.Query()
	for k := range maps.Keys(maskedQueryParams) {
		val := params.Get(k)
		if val == "" {
			continue
		}

		hashed := hash(val)
		params.Set(k, fmt.Sprintf("%d", hashed))
	}

	return fmt.Sprintf("%s?%s", strippedURI, params.Encode())
}

func hash(val string) uint64 {
	return xxhash.Sum64String(val)
}

// Logs an access event in Apache combined log format (with a minor customization with the duration).
// Additional allows to provide extra data that may be also logged, depending on the specific log format.
func LogAccess(entry *AccessEntry, additional map[string]interface{}) {
	if accessLog == nil || entry == nil {
		return
	}

	host := ""
	method := ""
	uri := ""
	proto := ""
	referer := ""
	userAgent := ""
	requestedHost := ""
	flowId := ""
	auditHeader := ""

	ts := entry.RequestTime.Format(dateFormat)
	status := entry.StatusCode
	responseSize := entry.ResponseSize
	duration := int64(entry.Duration / time.Millisecond)
	authUser := entry.AuthUser

	if entry.Request != nil {
		host = remoteHost(entry.Request)
		method = entry.Request.Method
		proto = entry.Request.Proto
		referer = entry.Request.Referer()
		userAgent = entry.Request.UserAgent()
		requestedHost = entry.Request.Host
		flowId = entry.Request.Header.Get(flowidFilter.HeaderName)

		uri = entry.Request.RequestURI
		if stripQuery {
			uri = stripQueryString(uri)
		} else if keys, ok := additional[KeyMaskedQueryParams].(map[string]bool); ok {
			if len(keys) > 0 {
				uri = maskQueryParams(entry.Request, keys)
			}
		}

		auditHeader = entry.Request.Header.Get(logFilter.UnverifiedAuditHeader)
	}

	logData := logrus.Fields{
		"timestamp":      ts,
		"host":           host,
		"method":         method,
		"uri":            uri,
		"proto":          proto,
		"referer":        referer,
		"user-agent":     userAgent,
		"status":         status,
		"response-size":  responseSize,
		"requested-host": requestedHost,
		"duration":       duration,
		"flow-id":        flowId,
		"audit":          auditHeader,
		"auth-user":      authUser,
	}

	delete(additional, KeyMaskedQueryParams)
	for k, v := range additional {
		logData[k] = v
	}

	logEntry := accessLog.WithFields(logData)
	if entry.Request != nil {
		logEntry = logEntry.WithContext(entry.Request.Context())
	}
	logEntry.Infoln()
}
