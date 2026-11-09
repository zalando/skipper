//go:build !awsnitro

package nitro

import "errors"

var errBuildTagRequired = errors.New("AWS Nitro support is not present; rebuild with -tags awsnitro")

// New returns an error when Nitro support was not included in the binary.
func New(Config) (Provider, error) {
	return nil, errBuildTagRequired
}
