//go:build gofuzz
// +build gofuzz

package fuzz

import "github.com/zalando/skipper/eskip"

func FuzzParsePredicates(data []byte) int {
	if _, err := eskip.ParsePredicates(string(data)); err != nil {
		return 0
	}

	return 1
}
