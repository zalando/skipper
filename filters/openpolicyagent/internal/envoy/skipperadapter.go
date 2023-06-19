package envoy

import (
	"net/http"
	"strings"

	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	_struct "github.com/golang/protobuf/ptypes/struct"
)

func AdaptToEnvoyExtAuthRequest(req *http.Request, pt PolicyType, contextExtensions map[string]string) *ext_authz_v3.CheckRequest {

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
			MetadataContext: &ext_authz_v3_core.Metadata{
				FilterMetadata: map[string]*_struct.Struct{
					"envoy.filters.http.header_to_metadata": {
						Fields: map[string]*_struct.Value{
							"policy_type": {
								Kind: &_struct.Value_StringValue{StringValue: string(pt)},
							},
						},
					},
				},
			},
		},
	}
}
