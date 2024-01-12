//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"fmt"
	"net/http"

	"github.com/zalando/skipper/predicates/source"
)

func FuzzSource(data []byte) int {
	p, err := source.New().Create([]interface{}{string(data)})

	if err != nil {
		return 0
	}

	if !p.Match(&http.Request{RemoteAddr: string(data)}) {
		panic(fmt.Sprintf("Source predicate match failed: %x\n", data))
	}

	if !p.Match(&http.Request{RemoteAddr: string(data), Header: http.Header{"X-Forwarded-For": []string{string(data)}}}) {
		panic(fmt.Sprintf("Source predicate with xff match failed: %x\n", data))
	}

	return 1
}
