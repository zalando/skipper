package envoy

import (
	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/protobuf/types/known/structpb"
	"net/http"
	"net/url"
	"reflect"
	"testing"
)

func TestAdaptToExtAuthRequest(t *testing.T) {
	tests := []struct {
		name              string
		req               *http.Request
		metadata          *ext_authz_v3_core.Metadata
		contextExtensions map[string]string
		rawBody           []byte
		want              *ext_authz_v3.CheckRequest
		wantErr           bool
	}{
		{
			name: "valid request with headers and metadata",
			req: &http.Request{
				Method: "GET",
				Host:   "example-app",
				URL:    &url.URL{Path: "/users/profile/charlie"},
				Header: createHeaders(map[string]string{
					"accept":            "*/*",
					"user-agent":        "curl/7.68.0",
					"x-request-id":      "1455bbb0-0623-4810-a2c6-df73ffd8863a",
					"x-forwarded-proto": "http",
				}),
			},
			metadata: createFilterMetadata(map[string]map[string]string{
				"envoy.filters.http.header_to_metadata": {"policy_type": "ingress"},
			}),
			contextExtensions: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			rawBody: []byte(`{"key":"value"}`),
			want: &ext_authz_v3.CheckRequest{
				Attributes: &ext_authz_v3.AttributeContext{
					Request: &ext_authz_v3.AttributeContext_Request{
						Http: &ext_authz_v3.AttributeContext_HttpRequest{
							Host:   "example-app",
							Method: "GET",
							Path:   "/users/profile/charlie",
							Headers: map[string]string{
								"accept":            "*/*",
								"user-agent":        "curl/7.68.0",
								"x-request-id":      "1455bbb0-0623-4810-a2c6-df73ffd8863a",
								"x-forwarded-proto": "http",
							},
							RawBody: []byte(`{"key":"value"}`),
						},
					},
					ContextExtensions: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					MetadataContext: createFilterMetadata(map[string]map[string]string{
						"envoy.filters.http.header_to_metadata": {"policy_type": "ingress"},
					}),
				},
			},
			wantErr: false,
		},
		//{
		//	name: "invalid request with malformed URL",
		//	req: &http.Request{
		//		Method: "GET",
		//		Host:   "example-app",
		//		URL:    &url.URL{Path: "/invalid/%C0%AF"},
		//		Header: nil,
		//	},
		//	metadata:          nil,
		//	contextExtensions: nil,
		//	rawBody:           nil,
		//	want:              nil,
		//	wantErr:           true,
		//},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AdaptToExtAuthRequest(tt.req, tt.metadata, tt.contextExtensions, tt.rawBody)
			if (err != nil) != tt.wantErr {
				t.Errorf("AdaptToExtAuthRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AdaptToExtAuthRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper to generate HTTP headers
func createHeaders(headerMap map[string]string) http.Header {
	headers := make(http.Header)
	for key, value := range headerMap {
		headers.Add(key, value)
	}
	return headers
}

// Helper to generate Envoy filter metadata
func createFilterMetadata(filterMap map[string]map[string]string) *ext_authz_v3_core.Metadata {
	filterMetadata := make(map[string]*structpb.Struct)
	for key, subMap := range filterMap {
		fields := make(map[string]*structpb.Value)
		for subKey, subValue := range subMap {
			fields[subKey] = structpb.NewStringValue(subValue)
		}
		filterMetadata[key] = &structpb.Struct{Fields: fields}
	}
	return &ext_authz_v3_core.Metadata{FilterMetadata: filterMetadata}
}
