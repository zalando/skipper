package envoy

import (
	"net/http"
	"strings"

	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
)

func AdaptToExtAuthRequest(req *http.Request, metadata *ext_authz_v3_core.Metadata, contextExtensions map[string]string) *ext_authz_v3.CheckRequest {

	headers := make(map[string]string)
	for h, vv := range req.Header {
		// This makes headers in the input compatible with what Envoy does, i.e. allows to use policy fragments designed for envoy
		// See: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/header_casing#http-1-1-header-casing
		headers[strings.ToLower(h)] = strings.Join(vv, ", ")
	}

	return &ext_authz_v3.CheckRequest{
		Attributes: &ext_authz_v3.AttributeContext{
			Request: &ext_authz_v3.AttributeContext_Request{
				Http: &ext_authz_v3.AttributeContext_HttpRequest{
					Host:    req.Host,
					Method:  req.Method,
					Path:    req.URL.Path,
					Headers: headers,
				},
			},
			ContextExtensions: contextExtensions,
			MetadataContext:   metadata,
		},
	}
}
