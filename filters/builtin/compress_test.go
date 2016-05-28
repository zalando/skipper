package builtin

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

const testBody = `
	<!doctype html>
	<html>
		<head>
			<meta charset="utf-8">
			<title>Hello-world page</title>
		</head>
		<body>
			<p>Hello, world!</p>
		</body>
	</html>
`

func Test(t *testing.T) {
	ctx := &filtertest.Context{
		FResponse: &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       ioutil.NopCloser(bytes.NewBufferString(testBody))}}
	s := NewCompress()
	f, err := s.CreateFilter(nil)
	if err != nil {
		t.Error(err)
		return
	}

	f.Response(ctx)
}
