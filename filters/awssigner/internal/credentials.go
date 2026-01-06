package awssigner

import (
	"path"
	"time"
)

// Credentials is the type to represent AWS credentials
type Credentials struct {
	// AccessKeyID is AWS Access key ID
	AccessKeyID string

	// SecretAccessKey is AWS Secret Access Key
	SecretAccessKey string

	// SessionToken is AWS Session Token
	SessionToken string

	// Source of the AWS credentials
	Source string

	// CanExpire states if the AWS credentials can expire or not.
	CanExpire bool

	// Expires is the time when the AWS credentials will expire. Should be ignored if CanExpire is false.
	Expires time.Time
}

// BuildCredentialScope builds part of credential string to be used as X-Amz-Credential header or query parameter.
func BuildCredentialScope(signingTime SigningTime, region, service string) string {
	return path.Join(
		signingTime.ShortTimeFormat(),
		region,
		service,
		"aws4_request")
}
