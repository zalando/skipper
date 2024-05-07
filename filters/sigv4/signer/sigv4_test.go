package signer

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
	"github.com/zalando/skipper/filters/filtertest"
	internal "github.com/zalando/skipper/filters/sigv4/internal/signer"
)

func TestSignRequest(t *testing.T) {
	var testCredentials = internal.Credentials{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"}
	req, body := buildRequest("dynamodb", "us-east-1", "{}")
	signer := NewSigner()
	ctx := &filtertest.Context{}
	err := signer.SignHTTP(ctx, testCredentials, req, body, "dynamodb", "us-east-1", time.Unix(0, 0))
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
	ctx := &filtertest.Context{}
	err := s.SignHTTP(ctx, testCredentials, req, bodyHash, "dynamodb", "us-east-1", time.Now())
	if err != nil {
		t.Fatalf("expect no error, got %v", err)
	}

	if req.Body != origBody {
		t.Errorf("expect request body to not be chagned")
	}
}

func TestRequestHost(t *testing.T) {
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
	sigV4 := sigV4Filter{
		region:                 "us-east-1",
		service:                "dynamodb",
		disableHeaderHoisting:  false,
		disableURIPathEscaping: false,
		disableSessionToken:    false,
	}

	//expectedDate := "19700101T000000Z"
	expectedSig := "AWS4-HMAC-SHA256 Credential=AKID/19700101/us-east-1/dynamodb/aws4_request, SignedHeaders=content-length;content-type;host;x-amz-date;x-amz-meta-other-header;x-amz-meta-other-header_with_underscore;x-amz-security-token;x-amz-target, Signature=a518299330494908a70222cec6899f6f32f297f8595f6df1776d998936652ad9"
	headers := &http.Header{}
	headers.Add("x-amz-accesskey", "AKID")
	headers.Add("x-amz-secret", "SECRET")
	headers.Add("x-amz-session", "SESSION")
	headers.Add("x-amz-time", time.Unix(0, 0).Format(time.RFC3339))

	headers.Add("X-Amz-Target", "prefix.Operation")
	headers.Add("Content-Type", "application/x-amz-json-1.0")
	headers.Add("X-Amz-Meta-Other-Header", "some-value=!@#$%^&* (+)")
	headers.Add("X-Amz-Meta-Other-Header_With_Underscore", "some-value=!@#$%^&* (+)")
	headers.Add("X-Amz-Meta-Other-Header_With_Underscore", "some-value=!@#$%^&* (+)")
	ctx := buildfilterContext(sigV4.service, sigV4.region, "POST", strings.NewReader("{}"), "//example.org/bucket/key-._~,!@#$%^&*()", *headers)
	sigV4.Request(ctx)
	signature := ctx.Request().Header[authorizationHeader][0]
	b, _ := io.ReadAll(ctx.Request().Body)
	print(string(b))
	assert.Equal(t, expectedSig, signature)

}

func buildfilterContext(serviceName string, region string, method string, body *strings.Reader, queryParams string, headers http.Header) filters.FilterContext {
	endpoint := "https://" + serviceName + "." + region + ".amazonaws.com"

	r, _ := http.NewRequest(method, endpoint, body)
	r.URL.Opaque = queryParams
	r.Header = headers
	return &filtertest.Context{FRequest: r}
}

func cmpDiff(e, a interface{}) string {
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
