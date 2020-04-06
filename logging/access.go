package logging

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	flowidFilter "github.com/zalando/skipper/filters/flowid"
	logFilter "github.com/zalando/skipper/filters/log"
	"github.com/zalando/skipper/remotehost"
)

const (
	dateFormat      = "02/Jan/2006:15:04:05 -0700"
	commonLogFormat = `%s - - [%s] "%s %s %s" %d %d`
	// format:
	// remote_host - - [date] "method uri protocol" status response_size "referer" "user_agent"
	combinedLogFormat = commonLogFormat + ` "%s" "%s"`
	// We add the duration in ms, a requested host and a flow id and audit log
	accessLogFormat = combinedLogFormat + " %d %s %s %s\n"
)

type accessLogFormatter struct {
	format string
}

// Access log entry.
type AccessEntry struct {

	// The client request.
	Request *http.Request

	// Reverse X-Forwarded-For header
	ReverseXForwardedForHeader bool

	// The status code of the response.
	StatusCode int

	// The size of the response in bytes.
	ResponseSize int64

	// The time spent processing request.
	Duration time.Duration

	// The time that the request was received.
	RequestTime time.Time
}

// TODO: create individual instances from the access log and
// delegate the ownership from the package level to the user
// code.
var (
	accessLog  *logrus.Logger
	stripQuery bool
)

func omitWhitespace(h string) string {
	if h != "" {
		return h
	}
	return "-"
}

func (f *accessLogFormatter) Format(e *logrus.Entry) ([]byte, error) {
	keys := []string{
		"host", "timestamp", "method", "uri", "proto",
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

// Logs an access event in Apache combined log format (with a minor customization with the duration).
// Additional allows to provide extra data that may be also logged, depending on the specific log format.
func LogAccess(entry *AccessEntry, additional map[string]interface{}) {
	if accessLog == nil || entry == nil {
		return
	}

	ts := entry.RequestTime.Format(dateFormat)

	host := ""
	method := ""
	uri := ""
	proto := ""
	referer := ""
	userAgent := ""
	requestedHost := ""
	flowId := ""
	var auditHeader string

	status := entry.StatusCode
	responseSize := entry.ResponseSize
	duration := int64(entry.Duration / time.Millisecond)

	if entry.Request != nil {
		if entry.ReverseXForwardedForHeader {
			if h := remotehost.RemoteHostFromLast(entry.Request); h != nil {
				host = h.String()
			}
		} else {
			if h := remotehost.RemoteHost(entry.Request); h != nil {
				host = h.String()
			}
		}
		method = entry.Request.Method
		proto = entry.Request.Proto
		referer = entry.Request.Referer()
		userAgent = entry.Request.UserAgent()
		requestedHost = entry.Request.Host
		flowId = entry.Request.Header.Get(flowidFilter.HeaderName)

		uri = entry.Request.RequestURI
		if stripQuery {
			uri = stripQueryString(uri)
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
	}

	for k, v := range additional {
		logData[k] = v
	}

	accessLog.WithFields(logData).Infoln()
}
