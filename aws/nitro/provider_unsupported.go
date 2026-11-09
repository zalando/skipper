//go:build awsnitro && !linux

package nitro

import "errors"

var errLinuxRequired = errors.New("AWS Nitro support is only available on Linux")

// New returns an error when Nitro support is requested on a non-Linux platform.
func New(Config) (Provider, error) {
	return nil, errLinuxRequired
}
