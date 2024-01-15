//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/zalando/skipper/predicates/query"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
)

func FuzzQuery(data []byte) int {
	f := fuzz.NewConsumer(data)

	args := struct{ k, v string }{}

	f.GenerateStruct(&args)

	if args.k == "" || args.v == "" {
		return 0
	}

	p, err := query.New().Create([]interface{}{args.k, args.v})

	if err != nil {
		return 0
	}

	if !p.Match(&http.Request{URL: &url.URL{RawQuery: args.k + "=" + args.v}}) {
		panic(fmt.Sprintf("Query predicate match failed: %x=%x\n", args.k, args.v))
	}

	return 1
}
