package awssigner

const AuthorizationHeader = "Authorization"
const doubleSpace = "  "

// AmzSignedHeadersKey is the set of headers signed for the request
const AmzSignedHeadersKey = "X-Amz-SignedHeaders"

// AmzCredentialKey is the access key ID and credential scope
const AmzCredentialKey = "X-Amz-Credential"

// TimeFormat is the time format to be used in the X-Amz-Date header or query parameter
const TimeFormat = "20060102T150405Z"

const SigningAlgorithm = "AWS4-HMAC-SHA256"

// ShortTimeFormat is the shorten time format used in the credential scope
const ShortTimeFormat = "20060102"

const AmzAlgorithmKey = "X-Amz-Algorithm"

const AmzDateKey = "X-Amz-Date"

// AmzSecurityTokenKey indicates the security token to be used with temporary credentials
const AmzSecurityTokenKey = "X-Amz-Security-Token"
