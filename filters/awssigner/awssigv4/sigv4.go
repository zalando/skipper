package awssigv4

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	internal "github.com/zalando/skipper/filters/awssigner/internal"
)

type awsSigV4Spec struct{}

const accessKeyHeader = "x-amz-accesskey"
const secretHeader = "x-amz-secret"
const sessionHeader = "x-amz-session"
const timeHeader = "x-amz-time"

type awsSigV4Filter struct {
	region                 string
	service                string
	disableHeaderHoisting  bool
	disableURIPathEscaping bool
	disableSessionToken    bool
}

func New() filters.Spec {
	return &awsSigV4Spec{}
}

func (*awsSigV4Spec) Name() string {
	return filters.AWSSigV4Name
}

func (c *awsSigV4Spec) CreateFilter(args []interface{}) (filters.Filter, error) {
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

	return &awsSigV4Filter{
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
func (f *awsSigV4Filter) Request(ctx filters.FilterContext) {
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
	sessionToken := ""
	if !f.disableSessionToken {
		sessionToken = getAndRemoveHeader(ctx, sessionHeader, req)
		if sessionToken == "" {
			return
		}
	}

	timeStr := getAndRemoveHeader(ctx, timeHeader, req)
	if timeStr == "" {
		return
	}

	time, err := time.Parse(time.RFC3339, timeStr)

	if err != nil {
		logger.Log(log.ErrorLevel, "time was not in RFC3339 format")
		return
	}

	hashedBody, body, err := hashRequest(ctx, req.Body)
	if err != nil {
		logger.Log(log.ErrorLevel, fmt.Sprintf("error occurred while hashing the body %s", err.Error()))
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
		options.DisableURIPathEscaping = f.disableURIPathEscaping
		options.Ctx = ctx.Request().Context()
	}
	//modifies request inplace
	signer.SignHTTP(creds, req, hashedBody, f.service, f.region, time, optfn)

	ctx.Request().Body = io.NopCloser(body) //ATTN: custom close() and read() set by skipper or previous filters are lost
}

func (f *awsSigV4Filter) Response(ctx filters.FilterContext) {}

func hashRequest(ctx filters.FilterContext, body io.Reader) (string, io.Reader, error) {
	h := sha256.New()
	if body == nil {
		body = http.NoBody
		_, err := io.Copy(h, body)
		if err != nil {

			return "", nil, err
		}
		return hex.EncodeToString(h.Sum(nil)), nil, nil
	} else {
		var buf bytes.Buffer
		tee := io.TeeReader(body, &buf)
		_, err := io.Copy(h, tee)
		if err != nil {
			return "", nil, err
		}
		return hex.EncodeToString(h.Sum(nil)), &buf, nil
	}
}

func getAndRemoveHeader(ctx filters.FilterContext, headerName string, req *http.Request) string {
	logger := log.WithContext(ctx.Request().Context())
	headerValue := req.Header.Get(headerName)
	if headerValue == "" {
		logger.Logf(log.ErrorLevel, "%q header is missing", headerName)
		return ""
	} else {
		req.Header.Del(headerName)
		return headerValue
	}
}
