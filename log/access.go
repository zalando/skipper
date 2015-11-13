package log

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"net"
	"net/http"
	"time"
)

const (
	dateFormat = "02/Jan/2006:15:04:05 -0700"
	// format:
	// remote_host - [date] "method uri protocol" status response_size "referer" "user_agent"
	accessLogFormat = `%s - [%s] "%s %s %s" %d %d "%s" "%s"`
)

type accessLogFormatter struct {
	format string
}

type AccessEntry struct {
	Request      *http.Request
	Response     *http.Response
	StatusCode   int
	ResponseSize int64
}

func remoteAddr(r *http.Request) string {
	// in case a proxy on the remote end sets the x-forwarded-for header,
	// then we may get meaningless ip addresses here. Input needed. On,
	// the other hand, Apache documentation,
	// https://httpd.apache.org/docs/1.3/logs.html#common, says that it
	// 'will be' the address of the proxy. If the proxy is the elb, or
	// something on our end, then that's not very interesting. The go
	// RemoteAddr field will be the proxy.

	// can this contain multiple values? what's the meaning of it then?
	// each proxy appends its own? no way to decide on the order.
	ff := r.Header.Get("X-Forwarded-For")
	if ff != "" {
		return ff
	}

	return r.RemoteAddr
}

// strip port from addresses with hostname, ipv4 or ipv6
func stripPort(address string) string {
	if h, _, err := net.SplitHostPort(address); err == nil {
		return h
	}

	return address
}

func remoteHost(r *http.Request) string {
	a := remoteAddr(r)
	h := stripPort(a)
	if h != "" {
		return h
	}

	return "-"
}

func timestamp() string {
	return time.Now().Format(dateFormat)
}

func getStatus(entry *AccessEntry) int {
	if entry.StatusCode != 0 {
		return entry.StatusCode
	}

	if entry.Response != nil && entry.Response.StatusCode != 0 {
		return entry.Response.StatusCode
	}

	return http.StatusNotFound
}

func (f *accessLogFormatter) Format(e *logrus.Entry) ([]byte, error) {
	keys := []string{
		"host", "timestamp", "method", "uri", "proto",
		"status", "response-size", "referer", "user-agent"}

	values := make([]interface{}, len(keys))
	for i, key := range keys {
		values[i] = e.Data[key]
	}

	return []byte(fmt.Sprintf(f.format, values...)), nil
}

func Access(entry *AccessEntry) {
	if accessLog == nil || entry == nil {
		return
	}

	ts := timestamp()

	host := "-"
	method := ""
	uri := ""
	proto := ""
	referer := ""
	userAgent := ""

	status := getStatus(entry)
	responseSize := entry.ResponseSize

	if entry.Request != nil {
		host = remoteHost(entry.Request)
		method = entry.Request.Method
		uri = entry.Request.RequestURI
		proto = entry.Request.Proto
		referer = entry.Request.Referer()
		userAgent = entry.Request.UserAgent()
	}

	accessLog.WithFields(logrus.Fields{
		"timestamp":     ts,
		"host":          host,
		"method":        method,
		"uri":           uri,
		"proto":         proto,
		"referer":       referer,
		"user-agent":    userAgent,
		"status":        status,
		"response-size": responseSize}).Info()
}
