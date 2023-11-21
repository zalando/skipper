//go:build gofuzz
// +build gofuzz

package fuzz

import "github.com/zalando/skipper/net"

func FuzzParseIPCIDRs(data []byte) int {
	if _, err := net.ParseIPCIDRs([]string{string(data)}); err != nil {
		return 0
	}

	return 1
}
