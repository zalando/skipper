package logging

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	flowidFilter "github.com/zalando/skipper/filters/flowid"
	logFilter "github.com/zalando/skipper/filters/log"
)

const (
	dateFormat      = "02/Jan/2006:15:04:05 -0700"
	commonLogFormat = `%s - - [%s] "%s %s %s" %d %d`
	// format:
	// remote_host - - [date] "method uri protocol" status response_size "referer" "user_agent"
	combinedLogFormat = commonLogFormat + ` "%s" "%s"`
	// We add the duration in ms, a requested host and a flow id and optional audit log
	accessLogFormat = combinedLogFormat + " %d %s %s%s\n"
)

type accessLogFormatter struct {
	format string
}

// Access log entry.
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
}

var accessLog *logrus.Logger

// strip port from addresses with hostname, ipv4 or ipv6
func stripPort(address string) string {
	if h, _, err := net.SplitHostPort(address); err == nil {
		return h
	}

	return address
}

// The remote address of the client. When the 'X-Forwarded-For'
// header is set, then it is used instead.
func remoteAddr(r *http.Request) string {
	ff := r.Header.Get("X-Forwarded-For")
	if ff != "" {
		return ff
	}

	return r.RemoteAddr
}

func remoteHost(r *http.Request) string {
	a := remoteAddr(r)
	h := stripPort(a)
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
		values[i] = e.Data[key]
	}

	return []byte(fmt.Sprintf(f.format, values...)), nil
}

// Logs an access event in Apache combined log format (with a minor customization with the duration).
func LogAccess(entry *AccessEntry) {
	if accessLog == nil || entry == nil {
		return
	}

	ts := entry.RequestTime.Format(dateFormat)

	host := "-"
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
		host = remoteHost(entry.Request)
		method = entry.Request.Method
		uri = entry.Request.RequestURI
		proto = entry.Request.Proto
		referer = entry.Request.Referer()
		userAgent = entry.Request.UserAgent()
		requestedHost = entry.Request.Host
		flowId = entry.Request.Header.Get(flowidFilter.HeaderName)
		auditHeader = entry.Request.Header.Get(logFilter.UnverifiedAuditHeader)
		if auditHeader != "" {
			auditHeader = " " + auditHeader
		}
	}

	accessLog.WithFields(logrus.Fields{
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
	}).Infoln()
}
