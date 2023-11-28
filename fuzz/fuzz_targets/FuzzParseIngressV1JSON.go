//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func FuzzParseIngressV1JSON(data []byte) int {
	if _, err := definitions.ParseIngressV1JSON(data); err != nil {
		return 0
	}

	return 1
}
