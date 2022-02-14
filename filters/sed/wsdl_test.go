package sed

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/filters/filtertest"
)

const (
	testWSDL    = "testdata/wsdl.xml"
	patchedWSDL = "testdata/wsdl-patched.xml"
)

func TestWSDLExample(t *testing.T) {
	response, err := os.ReadFile(testWSDL)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := os.ReadFile(patchedWSDL)
	if err != nil {
		t.Fatal(err)
	}

	resp := &http.Response{
		Body:          io.NopCloser(bytes.NewBuffer(response)),
		ContentLength: int64(len(response)),
	}

	sp := New()
	conf := []interface{}{
		"location=\"https?://[^/]+/ws/",
		"location=\"https://address-service.example.org/ws/",
	}

	f, err := sp.CreateFilter(conf)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FResponse: resp}
	f.Response(ctx)

	body, err := io.ReadAll(ctx.Response().Body)
	if err != nil {
		t.Error(err)
	}

	if l, hasContentLength := resp.Header["Content-Length"]; hasContentLength || resp.ContentLength != -1 {
		t.Error("Content-Length should not be set.", l, resp.ContentLength)
	}

	if !bytes.Equal(body, expected) {
		t.Error("Failed to receive the expected body.")
		t.Log(cmp.Diff(string(expected), string(body)))
	}
}
