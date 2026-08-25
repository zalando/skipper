package awssigv4

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	internal "github.com/zalando/skipper/filters/awssigner/internal"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestSignRequest(t *testing.T) {
	var testCredentials = internal.Credentials{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"}
	req, body := buildRequest("dynamodb", "us-east-1", "{}")
	signer := NewSigner()
	ctx := &filtertest.Context{
		FRequest: &http.Request{},
	}
	optfn := func(options *SignerOptions) {
		options.Ctx = ctx.FRequest.Context()
	}
	err := signer.SignHTTP(testCredentials, req, body, "dynamodb", "us-east-1", time.Unix(0, 0), optfn)
	if err != nil {
		t.Fatalf("expect no error, got %v", err)
	}

	expectedDate := "19700101T000000Z"
	expectedSig := "AWS4-HMAC-SHA256 Credential=AKID/19700101/us-east-1/dynamodb/aws4_request, SignedHeaders=content-length;content-type;host;x-amz-date;x-amz-meta-other-header;x-amz-meta-other-header_with_underscore;x-amz-security-token;x-amz-target, Signature=a518299330494908a70222cec6899f6f32f297f8595f6df1776d998936652ad9"

	q := req.Header
	if e, a := expectedSig, q.Get("Authorization"); e != a {
		t.Errorf("expect %v, got %v", e, a)
	}
	if e, a := expectedDate, q.Get("X-Amz-Date"); e != a {
		t.Errorf("expect %v, got %v", e, a)
	}
}

func TestBuildCanonicalRequest(t *testing.T) {
	req, _ := buildRequest("dynamodb", "us-east-1", "{}")
	req.URL.RawQuery = "Foo=z&Foo=o&Foo=m&Foo=a"

	ctx := &httpSigner{
		ServiceName:  "dynamodb",
		Region:       "us-east-1",
		Request:      req,
		Time:         internal.NewSigningTime(time.Now()),
		KeyDerivator: internal.NewSigningKeyDeriver(),
	}

	build, err := ctx.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := "https://example.org/bucket/key-._~,!@#$%^&*()?Foo=a&Foo=m&Foo=o&Foo=z"
	if e, a := expected, build.Request.URL.String(); e != a {
		t.Errorf("expect %v, got %v", e, a)
	}
}

func TestSigner_SignHTTP_NoReplaceRequestBody(t *testing.T) {
	req, bodyHash := buildRequest("dynamodb", "us-east-1", "{}")
	req.Body = io.NopCloser(bytes.NewReader([]byte{}))
	var testCredentials = internal.Credentials{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"}
	s := NewSigner()

	origBody := req.Body
	ctx := &filtertest.Context{
		FRequest: &http.Request{
			Header: http.Header{
				"X-Foo": []string{"foo"},
			},
		},
	}
	optfn := func(options *SignerOptions) {
		options.Ctx = ctx.FRequest.Context()
	}
	err := s.SignHTTP(testCredentials, req, bodyHash, "dynamodb", "us-east-1", time.Now(), optfn)
	if err != nil {
		t.Fatalf("expect no error, got %v", err)
	}

	if req.Body != origBody {
		t.Errorf("expect request body to not be chagned")
	}
}

/*
This test is being skipped since for skipper, we cannot dervive AWS host from req.
see https://github.com/zalando/skipper/pull/3070/files#diff-59e00c1e2a1a8ea3f9e5b4111f5c0b56cd7f81b1b14d8148f1dae146958d2c45R154
for change. We still keep this test to debug any unusual behaviours
*/
func TestRequestHost(t *testing.T) {
	t.Skip()
	req, _ := buildRequest("dynamodb", "us-east-1", "{}")
	req.URL.RawQuery = "Foo=z&Foo=o&Foo=m&Foo=a"
	req.Host = "myhost"

	query := req.URL.Query()
	query.Set("X-Amz-Expires", "5")
	req.URL.RawQuery = query.Encode()

	ctx := &httpSigner{
		ServiceName:  "dynamodb",
		Region:       "us-east-1",
		Request:      req,
		Time:         internal.NewSigningTime(time.Now()),
		KeyDerivator: internal.NewSigningKeyDeriver(),
	}

	build, err := ctx.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(build.CanonicalString, "host:"+req.Host) {
		t.Errorf("canonical host header invalid")
	}
}

func TestSign_buildCanonicalHeadersContentLengthPresent(t *testing.T) {
	body := `{"description": "this is a test"}`
	req, _ := buildRequest("dynamodb", "us-east-1", body)
	req.URL.RawQuery = "Foo=z&Foo=o&Foo=m&Foo=a"
	req.Host = "myhost"

	contentLength := fmt.Sprintf("%d", len([]byte(body)))
	req.Header.Add("Content-Length", contentLength)

	query := req.URL.Query()
	query.Set("X-Amz-Expires", "5")
	req.URL.RawQuery = query.Encode()

	ctx := &httpSigner{
		ServiceName:  "dynamodb",
		Region:       "us-east-1",
		Request:      req,
		Time:         internal.NewSigningTime(time.Now()),
		KeyDerivator: internal.NewSigningKeyDeriver(),
	}

	build, err := ctx.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(build.CanonicalString, "content-length:"+contentLength+"\n") {
		t.Errorf("canonical header content-length invalid")
	}
}

func TestSign_buildCanonicalHeaders(t *testing.T) {
	serviceName := "mockAPI"
	region := "mock-region"
	endpoint := "https://" + serviceName + "." + region + ".amazonaws.com"

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		t.Fatalf("failed to create request, %v", err)
	}

	req.Header.Set("FooInnerSpace", "   inner      space    ")
	req.Header.Set("FooLeadingSpace", "    leading-space")
	req.Header.Add("FooMultipleSpace", "no-space")
	req.Header.Add("FooMultipleSpace", "\ttab-space")
	req.Header.Add("FooMultipleSpace", "trailing-space    ")
	req.Header.Set("FooNoSpace", "no-space")
	req.Header.Set("FooTabSpace", "\ttab-space\t")
	req.Header.Set("FooTrailingSpace", "trailing-space    ")
	req.Header.Set("FooWrappedSpace", "   wrapped-space    ")

	ctx := &httpSigner{
		ServiceName:  serviceName,
		Region:       region,
		Request:      req,
		Time:         internal.NewSigningTime(time.Date(2021, 10, 20, 12, 42, 0, 0, time.UTC)),
		KeyDerivator: internal.NewSigningKeyDeriver(),
	}

	build, err := ctx.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectCanonicalString := strings.Join([]string{
		`POST`,
		`/`,
		``,
		`fooinnerspace:inner space`,
		`fooleadingspace:leading-space`,
		`foomultiplespace:no-space,tab-space,trailing-space`,
		`foonospace:no-space`,
		`footabspace:tab-space`,
		`footrailingspace:trailing-space`,
		`foowrappedspace:wrapped-space`,
		`host:mockAPI.mock-region.amazonaws.com`,
		`x-amz-date:20211020T124200Z`,
		``,
		`fooinnerspace;fooleadingspace;foomultiplespace;foonospace;footabspace;footrailingspace;foowrappedspace;host;x-amz-date`,
		``,
	}, "\n")
	if diff := cmpDiff(expectCanonicalString, build.CanonicalString); diff != "" {
		t.Errorf("expect match, got\n%s", diff)
	}
}

func TestSigV4(t *testing.T) {
	sigV4 := awsSigV4Filter{
		region:                 "us-east-1",
		service:                "dynamodb",
		disableHeaderHoisting:  false,
		disableURIPathEscaping: false,
		disableSessionToken:    false,
	}

	tests := []struct {
		accessKey         string
		secret            string
		session           string
		timeOfSigning     string
		expectedSignature string
		name              string
	}{
		{
			accessKey:         "",
			secret:            "some-invalid-secret",
			session:           "some-invalid-session",
			timeOfSigning:     "2012-11-01T22:08:41+00:00",
			expectedSignature: "",
			name:              "No_access_key_supplied",
		},
		{
			accessKey:         "some-invalid-accesskey",
			secret:            "",
			session:           "some-invalid-session",
			timeOfSigning:     "2012-11-01T22:08:41+00:00",
			expectedSignature: "",
			name:              "No_secret_key_provided",
		},
		{
			accessKey:         "some-access-key",
			secret:            "some-invalid-secret",
			session:           "",
			timeOfSigning:     "2012-11-01T22:08:41+00:00",
			expectedSignature: "",
			name:              "No_session_key_supplied",
		},
		{
			accessKey:         "some-access-key",
			secret:            "some-invalid-secret",
			session:           "some-invalid-session",
			timeOfSigning:     "",
			expectedSignature: "",
			name:              "No_time_of_signing_supplied",
		},
		{
			accessKey:         "some-access-key",
			secret:            "some-invalid-secret",
			session:           "some-invalid-session",
			timeOfSigning:     "2012-11-01T22:08:41+00",
			expectedSignature: "",
			name:              "incorrect_format_of_time_of_signing_supplied",
		},
		{
			accessKey:         "AKID",
			secret:            "SECRET",
			session:           "SESSION",
			timeOfSigning:     "1970-01-01T00:00:00Z",
			expectedSignature: "AWS4-HMAC-SHA256 Credential=AKID/19700101/us-east-1/dynamodb/aws4_request, SignedHeaders=content-length;content-type;host;x-amz-date;x-amz-meta-other-header;x-amz-meta-other-header_with_underscore;x-amz-security-token;x-amz-target, Signature=a518299330494908a70222cec6899f6f32f297f8595f6df1776d998936652ad9",
			name:              "all_headers_supplied",
		},
	}

	for _, test := range tests {
		expectedSig := test.expectedSignature
		headers := &http.Header{}
		headers.Add("x-amz-accesskey", test.accessKey)
		headers.Add("x-amz-secret", test.secret)
		headers.Add("x-amz-session", test.session)
		headers.Add("x-amz-time", test.timeOfSigning)

		headers.Add("X-Amz-Target", "prefix.Operation")
		headers.Add("Content-Type", "application/x-amz-json-1.0")
		headers.Add("X-Amz-Meta-Other-Header", "some-value=!@#$%^&* (+)")
		headers.Add("X-Amz-Meta-Other-Header_With_Underscore", "some-value=!@#$%^&* (+)")
		headers.Add("X-Amz-Meta-Other-Header_With_Underscore", "some-value=!@#$%^&* (+)")
		ctx := buildfilterContext(sigV4.service, sigV4.region, "POST", strings.NewReader("{}"), "//example.org/bucket/key-._~,!@#$%^&*()", *headers)

		sigV4.Request(ctx)

		signature := ctx.Request().Header.Get(internal.AuthorizationHeader)
		b, _ := io.ReadAll(ctx.Request().Body)
		assert.Equal(t, string(b), "{}") // test that body remains intact
		assert.Equal(t, expectedSig, signature, fmt.Sprintf("%s - test failed", test.name))
	}

}

func TestSigV4WithDisabledSessionToken(t *testing.T) {

	tests := []struct {
		accessKey           string
		secret              string
		session             string
		timeOfSigning       string
		expectedSignature   string
		name                string
		disableSessionToken bool
	}{
		{
			accessKey:           "some-token",
			secret:              "some-invalid-secret",
			session:             "",
			timeOfSigning:       "2012-11-01T22:08:41+00:00",
			expectedSignature:   "",
			disableSessionToken: false,
			name:                "session_token_expected_but_not_supplied",
		},
		{
			accessKey:           "AKID",
			secret:              "SECRET",
			session:             "",
			timeOfSigning:       "1970-01-01T00:00:00Z",
			expectedSignature:   "AWS4-HMAC-SHA256 Credential=AKID/19700101/us-east-1/dynamodb/aws4_request, SignedHeaders=content-length;content-type;host;x-amz-date;x-amz-meta-other-header;x-amz-meta-other-header_with_underscore;x-amz-session;x-amz-target, Signature=70ec16babd243ae915f100d0b63d5a0da2ff63c31d8631f1048b0441ab26743a",
			disableSessionToken: true,
			name:                "session_token_not_expected_and_not_supplied", // x-amz-session header is treated as normal header and used to calculate signature
		},
		{
			accessKey:           "AKID",
			secret:              "SECRET",
			session:             "SESSION",
			timeOfSigning:       "1970-01-01T00:00:00Z",
			expectedSignature:   "AWS4-HMAC-SHA256 Credential=AKID/19700101/us-east-1/dynamodb/aws4_request, SignedHeaders=content-length;content-type;host;x-amz-date;x-amz-meta-other-header;x-amz-meta-other-header_with_underscore;x-amz-security-token;x-amz-target, Signature=a4366bfb9558097b242ac243bf0e099267cbb657362495b5031c509644b2c3e9",
			disableSessionToken: false,
			name:                "session_token_expected_and_supplied", // x-amz-session header is treated as session header and is removed after reading
		},
		{
			accessKey:           "AKID",
			secret:              "SECRET",
			session:             "SESSION",
			timeOfSigning:       "1970-01-01T00:00:00Z",
			expectedSignature:   "AWS4-HMAC-SHA256 Credential=AKID/19700101/us-east-1/dynamodb/aws4_request, SignedHeaders=content-length;content-type;host;x-amz-date;x-amz-meta-other-header;x-amz-meta-other-header_with_underscore;x-amz-session;x-amz-target, Signature=2ee2d824eaead96cdec798f87530ce5269b59679038945608cfedf2d3622ad86",
			disableSessionToken: true,
			name:                "session_token_not_expected_and_supplied", // x-amz-session header is treated as normal header and used to calculate signature
		},
	}
	for _, test := range tests {
		sigV4 := awsSigV4Filter{
			region:                 "us-east-1",
			service:                "dynamodb",
			disableHeaderHoisting:  false,
			disableURIPathEscaping: false,
		}
		sigV4.disableSessionToken = test.disableSessionToken
		expectedSig := test.expectedSignature
		headers := &http.Header{}
		headers.Add("x-amz-accesskey", test.accessKey)
		headers.Add("x-amz-secret", test.secret)
		headers.Add("x-amz-session", test.session)
		headers.Add("x-amz-time", test.timeOfSigning)

		headers.Add("X-Amz-Target", "prefix.Operation")
		headers.Add("Content-Type", "application/x-amz-json-1.0")
		headers.Add("X-Amz-Meta-Other-Header", "some-value=!@#$%^&* (+)")
		headers.Add("X-Amz-Meta-Other-Header_With_Underscore", "some-value=!@#$%^&* (+)")
		headers.Add("X-Amz-Meta-Other-Header_With_Underscore", "some-value=!@#$%^&* (+)")
		ctx := buildfilterContext(sigV4.service, sigV4.region, "POST", strings.NewReader("{}"), "", *headers)

		sigV4.Request(ctx)

		signature := ctx.Request().Header.Get(internal.AuthorizationHeader)
		b, _ := io.ReadAll(ctx.Request().Body)
		assert.Equal(t, string(b), "{}") // test that body remains intact
		assert.Equal(t, expectedSig, signature, fmt.Sprintf("%s - test failed", test.name))
	}

}

func TestSigV4WithNoBody(t *testing.T) {
	sigV4 := awsSigV4Filter{
		region:                 "us-east-1",
		service:                "dynamodb",
		disableHeaderHoisting:  false,
		disableURIPathEscaping: false,
		disableSessionToken:    false,
	}
	expectedSignature := "AWS4-HMAC-SHA256 Credential=AKID/19700101/us-east-1/dynamodb/aws4_request, SignedHeaders=host;x-amz-date;x-amz-security-token, Signature=f779ebb258e1692ca97a514c3a9fb99ad1b6f8869608f43ff66bc00d2c0400f5"
	headers := &http.Header{}
	headers.Add("x-amz-accesskey", "AKID")
	headers.Add("x-amz-secret", "SECRET")
	headers.Add("x-amz-session", "SESSION")
	headers.Add("x-amz-time", "1970-01-01T00:00:00Z")
	ctx := buildfilterContext(sigV4.service, sigV4.region, "POST", strings.NewReader(""), "", *headers)
	sigV4.Request(ctx)
	signature := ctx.Request().Header.Get(internal.AuthorizationHeader)
	b, _ := io.ReadAll(ctx.Request().Body)
	assert.Equal(t, string(b), "") // test that body remains intact
	assert.Equal(t, expectedSignature, signature, "test with no body has failed")
}

func buildfilterContext(serviceName string, region string, method string, body *strings.Reader, queryParams string, headers http.Header) filters.FilterContext {
	endpoint := "https://" + serviceName + "." + region + ".amazonaws.com"

	r, _ := http.NewRequest(method, endpoint, body)
	r.URL.Opaque = queryParams
	r.Header = headers
	return &filtertest.Context{FRequest: r}
}

func cmpDiff(e, a any) string {
	if !reflect.DeepEqual(e, a) {
		return fmt.Sprintf("%v != %v", e, a)
	}
	return ""
}

func buildRequestWithBodyReader(serviceName, region string, body io.Reader) (*http.Request, string) {
	var bodyLen int

	type lenner interface {
		Len() int
	}
	if lr, ok := body.(lenner); ok {
		bodyLen = lr.Len()
	}

	endpoint := "https://" + serviceName + "." + region + ".amazonaws.com"
	req, _ := http.NewRequest("POST", endpoint, body)
	req.URL.Opaque = "//example.org/bucket/key-._~,!@#$%^&*()"
	req.Header.Set("X-Amz-Target", "prefix.Operation")
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")

	if bodyLen > 0 {
		req.ContentLength = int64(bodyLen)
	}

	req.Header.Set("X-Amz-Meta-Other-Header", "some-value=!@#$%^&* (+)")
	req.Header.Add("X-Amz-Meta-Other-Header_With_Underscore", "some-value=!@#$%^&* (+)")
	req.Header.Add("X-amz-Meta-Other-Header_With_Underscore", "some-value=!@#$%^&* (+)")

	h := sha256.New()
	_, _ = io.Copy(h, body)
	payloadHash := hex.EncodeToString(h.Sum(nil))

	return req, payloadHash
}

func buildRequest(serviceName, region, body string) (*http.Request, string) {
	reader := strings.NewReader(body)
	return buildRequestWithBodyReader(serviceName, region, reader)
}
