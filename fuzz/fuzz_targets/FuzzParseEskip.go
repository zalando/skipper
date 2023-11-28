//go:build gofuzz
// +build gofuzz

package fuzz

import "github.com/zalando/skipper/eskip"

func FuzzParseEskip(data []byte) int {
	if _, err := eskip.Parse(string(data)); err != nil {
		return 0
	}

	return 1
}
