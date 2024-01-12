//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"net/http"

	"github.com/zalando/skipper/predicates/host"
)

func FuzzHostAnyCreate(data []byte) int {
	if _, err := host.NewAny().Create([]interface{}{data}); err != nil {
		return 0
	}

	return 1
}

func FuzzHostAnyMatch(data []byte) int {
	p, err := host.NewAny().Create([]interface{}{string(data)})

	if err != nil {
		return 0
	}

	if !p.Match(&http.Request{Host: string(data)}) {
		panic("HostAny predicate match failed")
	}

	return 1
}
