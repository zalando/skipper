package envoy

import (
	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
	"net/http"
	"net/url"
	"reflect"
	"strings"
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
				URL:    &url.URL{Path: "/users/profile/amal#segment?param=yes"},
				Header: createHeaders(map[string]string{
					"accept":            "*/*",
					"user-agent":        "curl/7.68.0",
					"x-request-id":      "1455bbb0-0623-4810-a2c6-df73ffd8863a",
					"x-forwarded-proto": "http",
					":authority":        "example-app",
				}),
				Proto:         "HTTP/1.1",
				ContentLength: 100,
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
							Path:   "/users/profile/amal%23segment%3Fparam=yes", //URL encoded
							Headers: map[string]string{
								"accept":            "*/*",
								"user-agent":        "curl/7.68.0",
								"x-request-id":      "1455bbb0-0623-4810-a2c6-df73ffd8863a",
								"x-forwarded-proto": "http",
								":authority":        "example-app",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AdaptToExtAuthRequest(tt.req, tt.metadata, tt.contextExtensions, tt.rawBody)

			// Assert error
			assert.Equal(t, tt.wantErr, err != nil, "Unexpected error state")

			if err == nil {
				// Validate the transformed request using `want` fields
				assert.Equal(t, tt.want.Attributes.Request.Http.Host, got.Attributes.Request.Http.Host, "Mismatch in Host")
				assert.Equal(t, tt.want.Attributes.Request.Http.Method, got.Attributes.Request.Http.Method, "Mismatch in Method")
				assert.Equal(t, tt.want.Attributes.Request.Http.Path, got.Attributes.Request.Http.Path, "Mismatch in Path")

				// Headers comparison
				assert.Equal(t, tt.want.Attributes.Request.Http.Headers, got.Attributes.Request.Http.Headers, "Mismatch in Headers")

				// Metadata comparison
				assert.True(t, compareMetadata(tt.want.Attributes.MetadataContext, got.Attributes.MetadataContext), "Mismatch in MetadataContext")

				// Context extensions comparison
				assert.Equal(t, tt.want.Attributes.ContextExtensions, got.Attributes.ContextExtensions, "Mismatch in ContextExtensions")

				// RawBody comparison
				assert.Equal(t, tt.want.Attributes.Request.Http.RawBody, got.Attributes.Request.Http.RawBody, "Mismatch in RawBody")
			}
		})
	}
}

// Helper to compare Envoy metadata objects
func compareMetadata(expected *ext_authz_v3_core.Metadata, actual *ext_authz_v3_core.Metadata) bool {
	if len(expected.FilterMetadata) != len(actual.FilterMetadata) {
		return false
	}
	for key, expectedStruct := range expected.FilterMetadata {
		actualStruct, ok := actual.FilterMetadata[key]
		if !ok || !reflect.DeepEqual(expectedStruct.Fields, actualStruct.Fields) {
			return false
		}
	}
	return true
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

func Test_validateURLForInvalidUTF8(t *testing.T) {
	type args struct {
		u *url.URL
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid UTF-8 path and query",
			args:    args{u: parseURL(t, "https://example.com/path?query=value")},
			wantErr: false,
		},
		{
			name:    "invalid UTF-8 in path",
			args:    args{u: parseURL(t, "https://example.com/%C3%28")}, // Invalid UTF-8 sequence
			wantErr: true,
			errMsg:  `invalid utf8 in path: "/\xc3("`,
		},
		{
			name:    "valid UTF-8 path with invalid UTF-8 in query",
			args:    args{u: parseURL(t, "https://example.com/path?query=%C3%28")}, // Invalid UTF-8 in query
			wantErr: true,
			errMsg:  `invalid utf8 in query: "query=%C3%28"`,
		},
		{
			name:    "invalid UTF-8 in query and path",
			args:    args{u: parseURL(t, "https://example.com/%C3%28?query=%C3%28")},
			wantErr: true,
			errMsg:  `invalid utf8 in path: "/\xc3("`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURLForInvalidUTF8(tt.args.u)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURLForInvalidUTF8() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("validateURLForInvalidUTF8() error message = %v, expected to contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

// Helper function to parse a URL for testing
func parseURL(t *testing.T, rawurl string) *url.URL {
	u, err := url.Parse(rawurl)
	if err != nil {
		t.Fatalf("Failed to parse URL %s: %v", rawurl, err)
	}
	return u
}
