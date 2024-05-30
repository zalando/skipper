package awssigner

import (
	"path"
	"time"
)

// A Credentials is the AWS credentials value for individual credential fields.
type Credentials struct {
	// AWS Access key ID
	AccessKeyID string

	// AWS Secret Access Key
	SecretAccessKey string

	// AWS Session Token
	SessionToken string

	// Source of the credentials
	Source string

	// States if the credentials can expire or not.
	CanExpire bool

	// The time the credentials will expire at. Should be ignored if CanExpire
	// is false.
	Expires time.Time
}

func BuildCredentialScope(signingTime SigningTime, region, service string) string {
	return path.Join(
		signingTime.ShortTimeFormat(),
		region,
		service,
		"aws4_request")
}
