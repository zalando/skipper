//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"net/http"
	"net/url"

	"github.com/zalando/skipper/predicates/query"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
)

func FuzzQuery(data []byte) int {
	type Args struct {
		Key   string
		Value string
	}

	f := fuzz.NewConsumer(data)

	args := Args{}

	f.GenerateStruct(&args)

	p, err := query.New().Create([]interface{}{args.Key, args.Value})

	if err != nil {
		return 0
	}

	if !p.Match(&http.Request{URL: &url.URL{RawQuery: args.Key + "=" + args.Value}}) {
		panic("Query predicate match failed")
	}

	return 1
}
