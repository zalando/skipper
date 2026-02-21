package awssigv4

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	internal "github.com/zalando/skipper/filters/awssigner/internal"
)

type keyDerivator interface {
	DeriveKey(credential internal.Credentials, service, region string, signingTime internal.SigningTime) []byte
}

// Signer applies AWS v4 signing to given request. Use this to sign requests
// that need to be signed with AWS V4 Signatures.
type Signer struct {
	options      SignerOptions
	keyDerivator keyDerivator
}

type httpSigner struct {
	Request                *http.Request
	ServiceName            string
	Region                 string
	Time                   internal.SigningTime
	Credentials            internal.Credentials
	KeyDerivator           keyDerivator
	IsPreSign              bool
	PayloadHash            string
	DisableHeaderHoisting  bool
	DisableURIPathEscaping bool
	DisableSessionToken    bool
}

func NewSigner(optFns ...func(signer *SignerOptions)) *Signer {
	options := SignerOptions{}

	for _, fn := range optFns {
		fn(&options)
	}

	return &Signer{options: options, keyDerivator: internal.NewSigningKeyDeriver()}
}

func (s *httpSigner) buildCanonicalHeaders(host string, rule internal.Rule, header http.Header, length int64) (signed http.Header, signedHeaders, canonicalHeadersStr string) {
	signed = make(http.Header)

	var headers []string
	const hostHeader = "host"
	headers = append(headers, hostHeader)
	signed[hostHeader] = append(signed[hostHeader], host)

	const contentLengthHeader = "content-length"
	if length > 0 {
		headers = append(headers, contentLengthHeader)
		signed[contentLengthHeader] = append(signed[contentLengthHeader], strconv.FormatInt(length, 10))
	}

	for k, v := range header {
		if !rule.IsValid(k) {
			continue // ignored header
		}
		if strings.EqualFold(k, contentLengthHeader) {
			// prevent signing already handled content-length header.
			continue
		}

		lowerCaseKey := strings.ToLower(k)
		if _, ok := signed[lowerCaseKey]; ok {
			// include additional values
			signed[lowerCaseKey] = append(signed[lowerCaseKey], v...)
			continue
		}

		headers = append(headers, lowerCaseKey)
		signed[lowerCaseKey] = v
	}
	sort.Strings(headers)

	signedHeaders = strings.Join(headers, ";")

	var canonicalHeaders strings.Builder
	n := len(headers)
	const colon = ':'
	for i := range n {
		if headers[i] == hostHeader {
			canonicalHeaders.WriteString(hostHeader)
			canonicalHeaders.WriteRune(colon)
			canonicalHeaders.WriteString(internal.StripExcessSpaces(host))
		} else {
			canonicalHeaders.WriteString(headers[i])
			canonicalHeaders.WriteRune(colon)
			// Trim out leading, trailing, and dedup inner spaces from signed header values.
			values := signed[headers[i]]
			for j, v := range values {
				cleanedValue := strings.TrimSpace(internal.StripExcessSpaces(v))
				canonicalHeaders.WriteString(cleanedValue)
				if j < len(values)-1 {
					canonicalHeaders.WriteRune(',')
				}
			}
		}
		canonicalHeaders.WriteRune('\n')
	}
	canonicalHeadersStr = canonicalHeaders.String()

	return signed, signedHeaders, canonicalHeadersStr
}

func (s *httpSigner) Build() (signedRequest, error) {
	req := s.Request

	query := req.URL.Query()
	headers := req.Header

	s.setRequiredSigningFields(headers, query)

	// Sort Each Query Key's Values
	for key := range query {
		sort.Strings(query[key])
	}

	internal.SanitizeHostForHeader(req)

	credentialScope := s.buildCredentialScope()
	credentialStr := s.Credentials.AccessKeyID + "/" + credentialScope
	if s.IsPreSign {
		query.Set(internal.AmzCredentialKey, credentialStr)
	}

	unsignedHeaders := headers
	if s.IsPreSign && !s.DisableHeaderHoisting {
		var urlValues url.Values
		urlValues, unsignedHeaders = buildQuery(internal.AllowedQueryHoisting, headers)
		for k := range urlValues {
			query[k] = urlValues[k]
		}
	}
	//this is not valid way to extract host as skipper never receives aws host in req.URL. We should set it explicitly
	/*host := req.URL.Host
	if len(req.Host) > 0 {
		host = req.Host
	}*/
	host := s.ServiceName + "." + s.Region + ".amazonaws.com"

	signedHeaders, signedHeadersStr, canonicalHeaderStr := s.buildCanonicalHeaders(host, internal.IgnoredHeaders, unsignedHeaders, s.Request.ContentLength)

	if s.IsPreSign {
		query.Set(internal.AmzSignedHeadersKey, signedHeadersStr)
	}

	var rawQuery strings.Builder
	rawQuery.WriteString(strings.Replace(query.Encode(), "+", "%20", -1))

	canonicalURI := internal.GetURIPath(req.URL)
	if !s.DisableURIPathEscaping {
		canonicalURI = internal.EscapePath(canonicalURI, false)
	}

	canonicalString := s.buildCanonicalString(
		req.Method,
		canonicalURI,
		rawQuery.String(),
		signedHeadersStr,
		canonicalHeaderStr,
	)

	strToSign := s.buildStringToSign(credentialScope, canonicalString)
	signingSignature, err := s.buildSignature(strToSign)
	if err != nil {
		return signedRequest{}, err
	}

	if s.IsPreSign {
		rawQuery.WriteString("&X-Amz-Signature=")
		rawQuery.WriteString(signingSignature)
	} else {
		headers[internal.AuthorizationHeader] = append(headers[internal.AuthorizationHeader][:0], buildAuthorizationHeader(credentialStr, signedHeadersStr, signingSignature))
	}

	req.URL.RawQuery = rawQuery.String()

	return signedRequest{
		Request:         req,
		SignedHeaders:   signedHeaders,
		CanonicalString: canonicalString,
		StringToSign:    strToSign,
		PreSigned:       s.IsPreSign,
	}, nil

}

func (s *httpSigner) buildCanonicalString(method, uri, query, signedHeaders, canonicalHeaders string) string {
	return strings.Join([]string{
		method,
		uri,
		query,
		canonicalHeaders,
		signedHeaders,
		s.PayloadHash,
	}, "\n")
}

func (s *httpSigner) buildSignature(strToSign string) (string, error) {
	key := s.KeyDerivator.DeriveKey(s.Credentials, s.ServiceName, s.Region, s.Time)
	return hex.EncodeToString(internal.HMACSHA256(key, []byte(strToSign))), nil
}

type signedRequest struct {
	Request         *http.Request
	SignedHeaders   http.Header
	CanonicalString string
	StringToSign    string
	PreSigned       bool
}

func (s *httpSigner) buildStringToSign(credentialScope, canonicalRequestString string) string {
	return strings.Join([]string{
		internal.SigningAlgorithm,
		s.Time.TimeFormat(),
		credentialScope,
		hex.EncodeToString(makeHash(sha256.New(), []byte(canonicalRequestString))),
	}, "\n")
}

func makeHash(hash hash.Hash, b []byte) []byte {
	hash.Reset()
	hash.Write(b)
	return hash.Sum(nil)
}

func buildAuthorizationHeader(credentialStr, signedHeadersStr, signingSignature string) string {
	const credential = "Credential="
	const signedHeaders = "SignedHeaders="
	const signature = "Signature="
	const commaSpace = ", "

	var parts strings.Builder
	parts.Grow(len(internal.SigningAlgorithm) + 1 +
		len(credential) + len(credentialStr) + 2 +
		len(signedHeaders) + len(signedHeadersStr) + 2 +
		len(signature) + len(signingSignature),
	)
	parts.WriteString(internal.SigningAlgorithm)
	parts.WriteRune(' ')
	parts.WriteString(credential)
	parts.WriteString(credentialStr)
	parts.WriteString(commaSpace)
	parts.WriteString(signedHeaders)
	parts.WriteString(signedHeadersStr)
	parts.WriteString(commaSpace)
	parts.WriteString(signature)
	parts.WriteString(signingSignature)
	return parts.String()
}

func buildQuery(r internal.Rule, header http.Header) (url.Values, http.Header) {
	query := url.Values{}
	unsignedHeaders := http.Header{}
	for k, h := range header {
		if r.IsValid(k) {
			query[k] = h
		} else {
			unsignedHeaders[k] = h
		}
	}

	return query, unsignedHeaders
}

func (s *httpSigner) buildCredentialScope() string {
	return internal.BuildCredentialScope(s.Time, s.Region, s.ServiceName)
}

func (s *httpSigner) setRequiredSigningFields(headers http.Header, query url.Values) {
	amzDate := s.Time.TimeFormat()

	if s.IsPreSign {
		query.Set(internal.AmzAlgorithmKey, internal.SigningAlgorithm)
		sessionToken := s.Credentials.SessionToken
		if !s.DisableSessionToken && len(sessionToken) > 0 {
			query.Set("X-Amz-Security-Token", sessionToken)
		}

		query.Set(internal.AmzDateKey, amzDate)
		return
	}

	headers[internal.AmzDateKey] = append(headers[internal.AmzDateKey][:0], amzDate)

	if !s.DisableSessionToken && len(s.Credentials.SessionToken) > 0 {
		headers[internal.AmzSecurityTokenKey] = append(headers[internal.AmzSecurityTokenKey][:0], s.Credentials.SessionToken)
	}
}

// SignHTTP will modify the passed *http.Request in place.
func (s Signer) SignHTTP(credentials internal.Credentials, r *http.Request, payloadHash string, service string, region string, signingTime time.Time, optFns ...func(options *SignerOptions)) error {
	options := s.options

	for _, fn := range optFns {
		fn(&options)
	}

	signer := &httpSigner{
		Request:                r,
		PayloadHash:            payloadHash,
		ServiceName:            service,
		Region:                 region,
		Credentials:            credentials,
		Time:                   internal.NewSigningTime(signingTime.UTC()),
		DisableHeaderHoisting:  options.DisableHeaderHoisting,
		DisableURIPathEscaping: options.DisableURIPathEscaping,
		DisableSessionToken:    options.DisableSessionToken,
		KeyDerivator:           s.keyDerivator,
	}

	_, err := signer.Build()
	return err
}

func (s *httpSigner) BuildCanonicalString(method, uri, query, signedHeaders, canonicalHeaders string) string {
	return strings.Join([]string{
		method,
		uri,
		query,
		canonicalHeaders,
		signedHeaders,
		s.PayloadHash,
	}, "\n")
}

func (s *httpSigner) BuildStringToSign(credentialScope, canonicalRequestString string) string {
	return strings.Join([]string{
		internal.SigningAlgorithm,
		s.Time.TimeFormat(),
		credentialScope,
		hex.EncodeToString(makeHash(sha256.New(), []byte(canonicalRequestString))),
	}, "\n")
}

type SignerOptions struct {
	// Disables the Signer's moving HTTP header key/value pairs from the HTTP
	// request header to the request's query string. This is most commonly used
	// with pre-signed requests preventing headers from being added to the
	// request's query string.
	DisableHeaderHoisting bool

	// Disables the automatic escaping of the URI path of the request for the
	// signature's canonical string's path. For services that do not need additional
	// escaping then use this to disable the signer escaping the path.
	//
	// S3 is an example of a service that does not need additional escaping.
	//
	// http://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
	DisableURIPathEscaping bool

	// The logger to send log messages to.
	Ctx context.Context

	// Enable logging of signed requests.
	// This will enable logging of the canonical request, the string to sign, and for presigning the subsequent
	// presigned URL.
	LogSigning bool

	// Disables setting the session token on the request as part of signing
	// through X-Amz-Security-Token. This is needed for variations of v4 that
	// present the token elsewhere.
	DisableSessionToken bool
}
