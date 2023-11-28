//go:build gofuzz
// +build gofuzz

package fuzz

import "github.com/zalando/skipper/net"

func FuzzParseCIDRs(data []byte) int {
	if _, err := net.ParseCIDRs([]string{string(data)}); err != nil {
		return 0
	}

	return 1
}
