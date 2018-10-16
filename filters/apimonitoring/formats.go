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

type jsValue interface{}
type jsObject map[string]jsValue

func toJsonStringOrError(jso jsObject) string {
	jsStr, err := json.Marshal(jso)
	if err != nil {
		return err.Error()
	}
	return string(jsStr)
}

func mapFilterContext(c filters.FilterContext) jsValue {
	if c == nil {
		return nil
	}
	return jsObject{
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

func mapTracer(t opentracing.Tracer) jsValue {
	return fmt.Sprintf("%#v", t)
}

func mapMetrics(m filters.Metrics) jsValue {
	return fmt.Sprintf("%#v", m)
}

func mapResponseWriter(w http.ResponseWriter) jsValue {
	return fmt.Sprintf("%#v", w)
}

func mapHttpRequest(r *http.Request) jsValue {
	if r == nil {
		return nil
	}
	return jsObject{
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

func mapHeader(h http.Header) jsValue {
	result := make(jsObject)
	for k, v := range h {
		if headerIsNotBlacklisted(k) {
			result[k] = v
		}
	}
	return result
}

func mapPointer(p interface{}) jsValue {
	return fmt.Sprintf("Pointer: %p", p)
}

func mapConnectionState(s *tls.ConnectionState) jsValue {
	return fmt.Sprintf("%#v", s)
}

func mapForm(f *multipart.Form) jsValue {
	if f == nil {
		return nil
	}
	return jsObject{
		"Value": f.Value,
		"File":  f.File,
	}
}

func mapHttpResponse(r *http.Response) jsValue {
	if r == nil {
		return nil
	}
	return jsObject{
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

func mapUrl(url *url.URL) jsValue {
	if url == nil {
		return nil
	}
	return url.String()
}

func mapApiMonitoringFilter(f *apiMonitoringFilter) jsObject {
	if f == nil {
		return nil
	}
	return jsObject{
		"paths": mapPaths(f.paths),
	}
}

func mapPaths(infos []*pathInfo) []jsValue {
	result := make([]jsValue, len(infos))
	for i, p := range infos {
		result[i] = mapPath(p)
	}
	return result
}

func mapPath(info *pathInfo) jsValue {
	if info == nil {
		return nil
	}
	return jsObject{
		"path_template": info.PathTemplate,
		"matcher": info.Matcher.String(),
		"application_id": info.ApplicationId,
	}
}
