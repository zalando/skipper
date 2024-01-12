//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"fmt"
	"net/http"

	"github.com/zalando/skipper/predicates/host"
)

func FuzzHostAny(data []byte) int {
	p, err := host.NewAny().Create([]interface{}{string(data)})

	if err != nil {
		return 0
	}

	if !p.Match(&http.Request{Host: string(data)}) {
		panic(fmt.Sprintf("HostAny predicate match failed: %x\n", data))
	}

	return 1
}
