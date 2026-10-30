package net

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"net/http"
)

const internalRequestHeader = "Skipper-Internal-Request"

var internalRequestToken = rand.Text()

type internalRequestContextKey struct{}

// IsInternalRequest reports whether ingress authenticated this process's replay marker.
func IsInternalRequest(req *http.Request) bool {
	internal, _ := req.Context().Value(internalRequestContextKey{}).(bool)
	return internal
}

// MarkInternalRequest identifies an HTTP request replayed into this process.
func MarkInternalRequest(req *http.Request) {
	req.Header.Set(internalRequestHeader, internalRequestToken)
}

// ConsumeInternalRequest checks the process-local replay marker and removes it
// before the request can be forwarded or logged.
func ConsumeInternalRequest(req *http.Request) bool {
	value := req.Header.Get(internalRequestHeader)
	req.Header.Del(internalRequestHeader)
	internal := subtle.ConstantTimeCompare([]byte(value), []byte(internalRequestToken)) == 1
	if internal {
		*req = *req.WithContext(context.WithValue(req.Context(), internalRequestContextKey{}, true))
	}
	return internal
}
