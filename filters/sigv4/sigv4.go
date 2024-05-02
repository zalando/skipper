package sigv4

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/zalando/skipper/filters"
)

// TODO: add logging
// TODO: extract constants from code
type sigV4Spec struct{}
type sigV4Filter struct {
	accessKey string
	region    string
	service   string
}

func New() filters.Spec {
	return &sigV4Spec{}
}

func (*sigV4Spec) Name() string {
	return filters.SigV4
}

func (c *sigV4Spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 3 {
		return nil, filters.ErrInvalidFilterParameters
	}
	accessKey, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	region, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	service, ok := args[2].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &sigV4Filter{
		accessKey: accessKey,
		region:    region,
		service:   service,
	}, nil
}

/*
sigV4Filter is a request filter that signs the request.
In case a is non empty body is present in request,
the body is read to the maximum of bodySizeToBeRead value in 8kb chunks
and signed. The body is later reassigned to request. Operators should ensure
that bodySizeToBeRead is not more than the memory limit of skipper after taking
into accountthe concurrent requests.
*/
func (f *sigV4Filter) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	var body []byte
	if req.Body == nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			//TODO: handle error
		}
		generateSignature(ctx, body, f.accessKey, f.region, f.service)
	}
	ctx.Request().Body = io.NopCloser(bytes.NewReader(body)) //ATTN: custom close() and read() set by skipper or previous filters are lost
}

func (f *sigV4Filter) Response(ctx filters.FilterContext) {}

func generateSignature(ctx filters.FilterContext, body []byte, accessKey string, region string, service string) string {
	req := ctx.Request()
	method := req.Method
	endpoint := req.URL
	headers := map[string][]string(req.Header)
	queryparams := map[string][]string(req.URL.Query())

	canonicalRequest := generateCanonicalRequest(method, endpoint.RequestURI(), headers, queryparams, string(body))

	stringToSign := generateStringToSign(canonicalRequest, region, service)

	signingKey := generateSigningKey(accessKey, region, service)

	signature := calculateSignature(stringToSign, signingKey)

	return signature
}

func generateStringToSign(canonicalRequest string, region string, service string) string {
	return strings.Join([]string{
		"AWS4-HMAC-SHA256",
		time.Now().UTC().Format("20130524T000000Z"), //maybe headers["X-Amz-Date"]
		fmt.Sprintf("%s/%s/%s/aws4_request", time.Now().UTC().Format("20060102"), region, service),
		hexEncode(sHA256(canonicalRequest)),
	}, "\n")
}

func generateCanonicalEntities(entities map[string][]string, format string) []string {
	canonicalEntitySlice := make([]string, 0)
	entityKeys := make([]string, 0)

	for key, _ := range entities {
		entityKeys = append(entityKeys, key)
	}

	sort.Slice(entityKeys, func(i, j int) bool {
		return entityKeys[i] < entityKeys[j] // ascending order sort
	})

	for _, key := range entityKeys {
		key = strings.ToLower(key)
		sort.Strings(entities[key])                         // TODO: check specs
		queryParamValue := strings.Join(entities[key], ",") //TODO: check specs
		canonicalEntitySlice = append(canonicalEntitySlice, fmt.Sprintf(format, getEncodedPath(key, false), getEncodedPath(queryParamValue, false)))
	}
	return canonicalEntitySlice
}

func generateCanonicalRequest(method, endpoint string, headers map[string][]string, queryParams map[string][]string, payload string) string {
	var canonicalHeaders string
	var signedHeaders string

	// Sort headers and build canonical headers and signed headers
	canonicalHeadersSlice := generateCanonicalEntities(headers, "%s.%s")
	canonicalHeaders = strings.Join(canonicalHeadersSlice, "\n")

	canonicalQueryParamsSlice := generateCanonicalEntities(queryParams, "%s=%s")
	canonicalQueryParams := strings.Join(canonicalQueryParamsSlice, "&")

	canonicalPayLoad := hexEncode(sHA256(payload))

	canonicalRequest := strings.Join([]string{
		method,
		getEncodedPath(endpoint, false),
		canonicalQueryParams,
		canonicalHeaders,
		signedHeaders[:len(signedHeaders)-1], // TODO: find what is this in specs
		canonicalPayLoad,
	}, "\n")

	return canonicalRequest
}

func hexEncode(data string) string {
	src := []byte(data)
	return strings.ToLower(hex.EncodeToString(src))
}

func sHA256(data string) string {
	hash := sha256.New()
	hash.Write([]byte(data))
	return (string)(hash.Sum(nil))
}

func hmacSHA256(key string, data string) string {
	hmac := hmac.New(sha256.New, []byte(key))
	hmac.Write([]byte(data))
	return string(hmac.Sum(nil))
}

// amazon style encode of uri taken from https://github.com/aws/smithy-go/blob/v0.4.0/httpbinding/path_replace.go#L82
func getEncodedPath(path string, encodeSep bool) string {
	var noEscape [256]bool
	for i := 0; i < len(noEscape); i++ {
		// AWS expects every character except these to be escaped
		noEscape[i] = (i >= 'A' && i <= 'Z') ||
			(i >= 'a' && i <= 'z') ||
			(i >= '0' && i <= '9') ||
			i == '-' ||
			i == '.' ||
			i == '_' ||
			i == '~'
	}
	var buf bytes.Buffer
	for i := 0; i < len(path); i++ {
		c := path[i]
		if noEscape[c] || (c == '/' && !encodeSep) {
			buf.WriteByte(c)
		} else {
			fmt.Fprintf(&buf, "%%%02X", c)
		}
	}
	return buf.String()
}

func generateSigningKey(secretKey string, region string, service string) string {
	dateKey := hmacSHA256("AWS4"+secretKey, time.Now().UTC().Format("20060102"))
	dateRegionKey := hmacSHA256(dateKey, region)
	dateRegionServiceKey := hmacSHA256(dateRegionKey, service)
	signingKey := hmacSHA256(dateRegionServiceKey, "aws4_request")

	return signingKey
}

func calculateSignature(stringToSign string, signingKey string) string {
	return hexEncode(hmacSHA256(signingKey, stringToSign))
}
