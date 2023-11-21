//go:build gofuzz
// +build gofuzz

package fuzz

import "github.com/zalando/skipper/jwt"

func FuzzParseJwt(data []byte) int {
	if _, err := jwt.Parse(string(data)); err != nil {
		return 0
	}

	return 1
}
