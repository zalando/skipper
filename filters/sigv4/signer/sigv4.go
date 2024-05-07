package signer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/zalando/skipper/filters"
	internal "github.com/zalando/skipper/filters/sigv4/internal/signer"
)

// TODO: add logging
// TODO: extract constants from code
type sigV4Spec struct {
	DisableHeaderHoisting  bool
	DisableURIPathEscaping bool
	DisableSessionToken    bool
}

const signingAlgorithm = "AWS4-HMAC-SHA256"
const authorizationHeader = "Authorization"

type sigV4Filter struct {
	accessKey              string
	region                 string
	service                string
	disableHeaderHoisting  bool
	disableURIPathEscaping bool
	disableSessionToken    bool
	sessionToken           string
	secretKey              string
	time                   time.Time
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
		accessKey:              accessKey,
		region:                 region,
		service:                service,
		disableHeaderHoisting:  c.DisableHeaderHoisting,
		disableURIPathEscaping: c.DisableURIPathEscaping,
		disableSessionToken:    c.DisableSessionToken,
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

	signer := NewSigner()
	hashedBody, body := hashRequest(req.Body)
	creds := internal.Credentials{
		AccessKeyID:     f.accessKey,
		SessionToken:    f.sessionToken,
		SecretAccessKey: f.secretKey,
	}

	optfn := func(options *SignerOptions) {
		options.DisableHeaderHoisting = f.disableHeaderHoisting
		options.DisableSessionToken = f.disableSessionToken
		options.DisableURIPathEscaping = f.disableSessionToken
	}
	/*
		modifies request in-place
		sign time is now
	*/
	signer.SignHTTP(ctx, creds, req, hashedBody, f.service, f.region, f.time, optfn)

	ctx.Request().Body = io.NopCloser(bytes.NewReader(body)) //ATTN: custom close() and read() set by skipper or previous filters are lost
}

func (f *sigV4Filter) Response(ctx filters.FilterContext) {}

func hashRequest(body io.Reader) (string, []byte) {
	h := sha256.New()
	if body == nil {
		body = strings.NewReader("{}") // as per specs https://github.com/aws/aws-sdk-go-v2/blob/main/aws/signer/v4/v4.go#L261
	}
	var buf bytes.Buffer
	tee := io.TeeReader(body, &buf)
	_, _ = io.Copy(h, tee) // read
	b, err := io.ReadAll(&buf)
	if err != nil {
		// TODO:
	}
	return hex.EncodeToString(h.Sum(nil)), b
}
