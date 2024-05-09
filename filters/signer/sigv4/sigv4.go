package sigv4

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	internal "github.com/zalando/skipper/filters/signer/internal"
)

type sigV4Spec struct {
	// Disables the Signer's moving HTTP header key/value pairs from the HTTP
	// request header to the request's query string. This is most commonly used
	// with pre-signed requests preventing headers from being added to the
	// request's query string.
	disableHeaderHoisting bool

	// Disables the automatic escaping of the URI path of the request for the
	// siganture's canonical string's path. For services that do not need additional
	// escaping then use this to disable the signer escaping the path.
	//
	// S3 is an example of a service that does not need additional escaping.
	//
	// http://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
	disableURIPathEscaping bool

	// Disables setting the session token on the request as part of signing
	// through X-Amz-Security-Token. This is needed for variations of v4 that
	// present the token elsewhere.
	disableSessionToken bool
}

const accessKeyHeader = "x-amz-accesskey"
const secretHeader = "x-amz-secret"
const sessionHeader = "x-amz-session"
const timeHeader = "x-amz-time"

type sigV4Filter struct {
	region                 string
	service                string
	disableHeaderHoisting  bool
	disableURIPathEscaping bool
	disableSessionToken    bool
}

/*
	 	Disables the Signer's moving HTTP header key/value pairs from the HTTP
		request header to the request's query string. This is most commonly used
		with pre-signed requests preventing headers from being added to the
		request's query string.
		disableHeaderHoisting bool

		Disables the automatic escaping of the URI path of the request for the
		siganture's canonical string's path. For services that do not need additional
		escaping then use this to disable the signer escaping the path.

		S3 is an example of a service that does not need additional escaping.

		http://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
		disableURIPathEscaping bool

		Disables setting the session token on the request as part of signing
		through X-Amz-Security-Token. This is needed for variations of v4 that
		present the token elsewhere.
		disableSessionToken bool
*/
func New() filters.Spec {
	return &sigV4Spec{}
}

func (*sigV4Spec) Name() string {
	return filters.SigV4
}

func (c *sigV4Spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 5 {
		return nil, filters.ErrInvalidFilterParameters
	}

	region, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	service, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	disableHeaderHoistingStr, ok := args[2].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	disableURIPathEscapingStr, ok := args[3].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	disableSessionTokenStr, ok := args[4].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	disableHeaderHoisting, err := strconv.ParseBool(disableHeaderHoistingStr)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	disableURIPathEscaping, err := strconv.ParseBool(disableURIPathEscapingStr)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	disableSessionToken, err := strconv.ParseBool(disableSessionTokenStr)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &sigV4Filter{
		region:                 region,
		service:                service,
		disableHeaderHoisting:  disableHeaderHoisting,
		disableURIPathEscaping: disableURIPathEscaping,
		disableSessionToken:    disableSessionToken,
	}, nil
}

/*
sigV4Filter is a request filter that signs the request.
In case a non empty is body is present in request,
the body is read and signed. The body is later reassigned to request. Operators should ensure
that body size by all requests at any point of time is not more than the memory limit of skipper.
*/
func (f *sigV4Filter) Request(ctx filters.FilterContext) {
	req := ctx.Request()

	logger := log.WithContext(req.Context())

	signer := NewSigner()

	accessKey := getAndRemoveHeader(ctx, accessKeyHeader, req)
	if accessKey == "" {
		return
	}

	secretKey := getAndRemoveHeader(ctx, secretHeader, req)
	if secretKey == "" {
		return
	}

	sessionToken := getAndRemoveHeader(ctx, sessionHeader, req)
	if sessionToken == "" {
		return
	}

	timeStr := getAndRemoveHeader(ctx, timeHeader, req)
	if timeStr == "" {
		return
	}

	time, err := time.Parse(time.RFC3339, timeStr) //2012-11-01T22:08:41+00:00

	if err != nil {
		logger.Log(log.ErrorLevel, "time was not in RFC3339 format")
		return
	}

	hashedBody, body := hashRequest(ctx, req.Body)
	if hashedBody == "" {
		logger.Log(log.ErrorLevel, "error occured while hashing the body")
		return
	}
	creds := internal.Credentials{
		AccessKeyID:     accessKey,
		SessionToken:    sessionToken,
		SecretAccessKey: secretKey,
	}

	optfn := func(options *SignerOptions) {
		options.DisableHeaderHoisting = f.disableHeaderHoisting
		options.DisableSessionToken = f.disableSessionToken
		options.DisableURIPathEscaping = f.disableSessionToken
		options.Ctx = ctx.Request().Context()
	}
	//modifies request inplace
	signer.SignHTTP(creds, req, hashedBody, f.service, f.region, time, optfn)

	ctx.Request().Body = io.NopCloser(body) //ATTN: custom close() and read() set by skipper or previous filters are lost
}

func (f *sigV4Filter) Response(ctx filters.FilterContext) {}

func hashRequest(ctx filters.FilterContext, body io.Reader) (string, io.Reader) {
	logger := log.WithContext(ctx.Request().Context())
	h := sha256.New()
	if body == nil {
		body = strings.NewReader("{}") // as per specs https://github.com/aws/aws-sdk-go-v2/blob/main/aws/signer/v4/v4.go#L261
	}
	var buf bytes.Buffer
	tee := io.TeeReader(body, &buf)
	_, err := io.Copy(h, tee) // read body in-memory
	if err != nil {
		logger.Logf(log.DebugLevel, "an error occured while reading the body %v", err.Error())
		return "", nil
	}
	return hex.EncodeToString(h.Sum(nil)), &buf
}

func getAndRemoveHeader(ctx filters.FilterContext, headerName string, req *http.Request) string {
	logger := log.WithContext(ctx.Request().Context())
	headerValue := req.Header.Get(headerName)
	if headerValue == "" {
		logger.Logf(log.ErrorLevel, "%s header is missing", headerName)
		return ""
	} else {
		req.Header.Del(headerName)
		return headerValue
	}
}
