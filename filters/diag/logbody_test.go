package diag

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestLogBodyRequest(t *testing.T) {
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	bodies := []struct {
		contentType   string
		body          string
		containsError bool
	}{
		{"text/plain", "foo bar baz", false},
		{"application/json", `{"foo":"bar"}`, false},
		{"application/ld+json", `{"foo":"bar"}`, false},
		{"application/xml", `<foo>bar</foo>`, false},
		{"", "foo bar baz", true},
		{"image/gif", "foo bar baz", true},
		{"multipart/form-data", "foo bar baz", true},
		{"video/webm", "foo bar baz", true},
		{"application/vnd.mozilla.xul+xml", "foo bar baz", true},
	}

	req, err := http.NewRequest("GET", "https://example.org/", nil)
	res := &http.Response{
		Status: "200 OK",
		Header: http.Header{},
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range bodies {
		req.Body = io.NopCloser(bytes.NewBufferString(body.body))
		req.Header.Set("Content-Type", body.contentType)
		ctx := &filtertest.Context{
			FRequest:  req,
			FResponse: res,
		}

		loggerTest := bytes.NewBuffer(nil)
		log.SetOutput(loggerTest)
		lh, err := (logBody{}).CreateFilter([]interface{}{})
		if err != nil {
			t.Fatal(err)
		}

		lh.Request(ctx)
		t.Logf("loggerTest: %s", loggerTest.String())
		errorInLog := bytes.Contains(loggerTest.Bytes(), []byte("error"))
		pass := !body.containsError && !errorInLog || body.containsError && errorInLog
		if !pass {
			t.Fail()
		}
	}
}

func TestLogBodyResponse(t *testing.T) {
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	bodies := []struct {
		contentType   string
		body          string
		containsError bool
	}{
		{"text/plain", "foo bar baz", false},
		{"application/json", `{"foo":"bar"}`, false},
		{"application/ld+json", `{"foo":"bar"}`, false},
		{"application/xml", `<foo>bar</foo>`, false},
		{"", "foo bar baz", true},
		{"image/gif", "foo bar baz", true},
		{"multipart/form-data", "foo bar baz", true},
		{"video/webm", "foo bar baz", true},
		{"application/vnd.mozilla.xul+xml", "foo bar baz", true},
	}

	req, err := http.NewRequest("GET", "https://example.org/", nil)
	
	res := &http.Response{
		Status: "200 OK",
		Header: http.Header{},
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range bodies {
		res.Body = io.NopCloser(bytes.NewBufferString(body.body))
		res.Header.Set("Content-Type", body.contentType)
		ctx := &filtertest.Context{
			FRequest:  req,
			FResponse: res,
		}

		loggerTest := bytes.NewBuffer(nil)
		log.SetOutput(loggerTest)
		lh, err := (logBody{}).CreateFilter([]interface{}{"response"})
		
		if err != nil {
			t.Fatal(err)
		}
		lh.Response(ctx)
		t.Logf("loggerTest: %s", loggerTest.String())
		errorInLog := bytes.Contains(loggerTest.Bytes(), []byte("error"))
		pass := !body.containsError && !errorInLog || body.containsError && errorInLog
		if !pass {
			t.Fail()
		}
	}

}
