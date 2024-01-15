//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"fmt"
	"net/http"

	"github.com/zalando/skipper/predicates/content"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
)

func FuzzContentLengthBetween(data []byte) int {
	f := fuzz.NewConsumer(data)

	args := struct{ min, max int64 }{}

	f.GenerateStruct(&args)

	p, err := content.NewContentLengthBetween().Create([]interface{}{args.min, args.max})

	if err != nil {
		return 0
	}

	if !p.Match(&http.Request{ContentLength: int64(args.min)}) || !p.Match(&http.Request{ContentLength: int64(args.max - 1)}) {
		panic(fmt.Sprintf("ContentLengthBetween predicate match failed: %x\n", data))
	}

	return 1
}
