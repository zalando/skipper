//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"fmt"
	"net/http"

	"github.com/zalando/skipper/predicates/source"
)

func FuzzClientIP(data []byte) int {
	p, err := source.NewClientIP().Create([]interface{}{string(data)})

	if err != nil {
		return 0
	}

	if !p.Match(&http.Request{RemoteAddr: string(data)}) {
		panic(fmt.Sprintf("ClientIP predicate match failed: %x\n", data))
	}

	if !p.Match(&http.Request{RemoteAddr: string(data), Header: http.Header{"X-Forwarded-For": []string{string(data)}}}) {
		panic(fmt.Sprintf("ClientIP predicate with xff match failed: %x\n", data))
	}

	return 1
}
