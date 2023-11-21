//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func FuzzParseRouteGroupsJSON(data []byte) int {
	if _, err := definitions.ParseRouteGroupsJSON(data); err != nil {
		return 0
	}

	return 1
}
