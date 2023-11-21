//go:build gofuzz
// +build gofuzz

package fuzz

import "github.com/zalando/skipper/eskip"

func FuzzParseFilters(data []byte) int {
	if _, err := eskip.ParseFilters(string(data)); err != nil {
		return 0
	}

	return 1
}
