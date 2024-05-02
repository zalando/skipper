package sigv4

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func Test_Sigv4Signature(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		headers     map[string][]string
		queryParams map[string][]string
		accessKey   string
		region      string
		service     string
		method      string
		baseUrl     string
		signature   string
		pathParams  string
	}{
		{
			body: "{}",
			headers: map[string][]string{
				"X-Amz-Meta-Other-Header":                 []string{"some-value=!@#$%^&* (+)"},
				"X-Amz-Meta-Other-Header_With_Underscore": []string{"some-value=!@#$%^&* (+)"},
				"X-amz-Meta-Other-Header_With_Underscore": []string{"some-value=!@#$%^&* (+)"},
				"X-Amz-Target":                            []string{"prefix.Operation"},
				"Content-Type":                            []string{"application/x-amz-json-1.0"},
			},
			queryParams: nil, //
			service:     "dynamodb",
			region:      "us-east-1",
			accessKey:   "AKID",
			method:      "POST",
			baseUrl:     "https://dynamodb.us-east-1.amazonaws.com", //no url encoding here
			pathParams:  "//example.org/bucket/key-._~,!@#$%^&*()",
			signature:   "a518299330494908a70222cec6899f6f32f297f8595f6df1776d998936652ad9",
		},
	}

	for _, test := range tests {
		ctx := buildfilterContext(test.method, test.baseUrl, test.body, test.pathParams, test.headers)
		signature := generateSignature(ctx, []byte(test.body), test.accessKey, test.region, test.service)
		assert.Equal(t, test.signature, signature, fmt.Sprintf("signature for test %s does not match expected value", test.name))
	}

}

func buildfilterContext(method string, baseurl string, body string, pathParams string, headers map[string][]string) filters.FilterContext {
	r, _ := http.NewRequest(method, baseurl, bytes.NewReader([]byte(body)))
	r.URL.Opaque = pathParams
	r.Header = headers
	return &filtertest.Context{FRequest: r}
}
