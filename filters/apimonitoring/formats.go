package apimonitoring

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"mime/multipart"
	"net/http"
	"net/url"
)

type JSValue interface{}
type JSObject map[string]JSValue

func formatFilterContext(c filters.FilterContext) string {
	jsMap := mapFilterContext(c)
	jsStr, err := json.Marshal(jsMap)
	if err != nil {
		panic(err)
	}
	return string(jsStr)
}

func mapFilterContext(c filters.FilterContext) JSValue {
	if c == nil {
		return nil
	}
	return JSObject{
		"ResponseWriter":   mapResponseWriter(c.ResponseWriter()),
		"Request":          mapHttpRequest(c.Request()),
		"Response":         mapHttpResponse(c.Response()),
		"OriginalRequest":  mapHttpRequest(c.OriginalRequest()),
		"OriginalResponse": mapHttpResponse(c.OriginalResponse()),
		"Served":           c.Served(),
		"StateBag":         c.StateBag(),
		"BackendUrl":       c.BackendUrl(),
		"OutgoingHost":     c.OutgoingHost(),
		"Metrics":          mapMetrics(c.Metrics()),
		"Tracer":           mapTracer(c.Tracer()),
	}
}

func mapTracer(t opentracing.Tracer) JSValue {
	if t == nil {
		return nil
	}
	return fmt.Sprintf("%#v", t)
}

func mapMetrics(m filters.Metrics) JSValue {
	if m == nil {
		return nil
	}
	return fmt.Sprintf("%#v", m)
}

func mapResponseWriter(w http.ResponseWriter) JSValue {
	if w == nil {
		return nil
	}
	return fmt.Sprintf("%#v", w)
}

func mapHttpRequest(r *http.Request) JSValue {
	if r == nil {
		return nil
	}
	return JSObject{
		"Method":           r.Method,
		"URL":              mapUrl(r.URL),
		"Proto":            r.Proto,
		"ProtoMajor":       r.ProtoMajor,
		"ProtoMinor":       r.ProtoMinor,
		"Header":           mapHeader(r.Header),
		"Body":             mapPointer(r.Body),
		"ContentLength":    r.ContentLength,
		"TransferEncoding": r.TransferEncoding,
		"Host":             r.Host,
		"Form":             r.Form,
		"PostForm":         r.PostForm,
		"MultipartForm":    mapForm(r.MultipartForm),
		"Trailer":          r.Trailer,
		"RemoteAddr":       r.RemoteAddr,
		"RequestURI":       r.RequestURI,
		"TLS":              mapConnectionState(r.TLS),
		"Response":         mapPointer(r.Response),
	}
}

var headerBlackList = []string{
	"Authorization",
	"Cookie",
	"Set-Cookie",
}

func headerIsNotBlacklisted(headerName string) bool {
	for _, blackListed := range headerBlackList {
		if headerName == blackListed {
			return false
		}
	}
	return true
}

func mapHeader(h http.Header) JSValue {
	result := make(JSObject)
	for k, v := range h {
		if headerIsNotBlacklisted(k) {
			result[k] = v
		}
	}
	return result
}

func mapPointer(p interface{}) JSValue {
	if p == nil {
		return nil
	}
	return fmt.Sprintf("Pointer: %p", p)
}

func mapConnectionState(s *tls.ConnectionState) JSValue {
	if s == nil {
		return nil
	}
	return fmt.Sprintf("%#v", s)
}

func mapForm(f *multipart.Form) JSValue {
	if f == nil {
		return nil
	}
	return JSObject{
		"Value": f.Value,
		"File":  f.File,
	}
}

func mapHttpResponse(r *http.Response) JSValue {
	if r == nil {
		return nil
	}
	return JSObject{
		"Status":           r.Status,
		"StatusCode":       r.StatusCode,
		"Proto":            r.Proto,
		"ProtoMajor":       r.ProtoMajor,
		"ProtoMinor":       r.ProtoMinor,
		"Header":           mapHeader(r.Header),
		"Body":             mapPointer(r.Body),
		"ContentLength":    r.ContentLength,
		"TransferEncoding": r.TransferEncoding,
		"Uncompressed":     r.Uncompressed,
		"Trailer":          r.Trailer,
		"Request":          mapPointer(r.Request),
		"TLS":              mapConnectionState(r.TLS),
	}
}

func mapUrl(url *url.URL) JSValue {
	if url == nil {
		return nil
	}
	return url.String()
}
