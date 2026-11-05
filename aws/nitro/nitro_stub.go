//go:build !awsnitro

// Package nitro provides a no-op stub for non-Nitro builds. See nitro.go for
// the real implementation, which requires the awsnitro build tag and CGO.
package nitro

import "crypto/tls"

// Config holds Nitro ACM parameters. In non-awsnitro builds this struct is
// present for API compatibility but TLSConfig always returns nil, nil.
type Config struct {
	Module     string
	TokenLabel string
	PIN        string
	CertFile   string
}

// TLSConfig is a no-op stub. It always returns nil, nil.
func TLSConfig(_ Config) (*tls.Config, error) { return nil, nil }
