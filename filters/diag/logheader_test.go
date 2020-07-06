package diag

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestLogHeader(t *testing.T) {
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	req, err := http.NewRequest("GET", "https://example.org", nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &filtertest.Context{
		FRequest: req,
	}

	outputVerify := bytes.NewBuffer(nil)
	req.Body = nil
	if err := req.Write(outputVerify); err != nil {
		t.Fatal(err)
	}

	loggerVerify := bytes.NewBuffer(nil)
	log.SetOutput(loggerVerify)
	log.Println(outputVerify.String())

	loggerTest := bytes.NewBuffer(nil)
	log.SetOutput(loggerTest)

	req.Body = ioutil.NopCloser(bytes.NewBufferString("foo bar baz"))
	(logHeader{}).Request(ctx)
	if loggerTest.Len() == 0 || !bytes.Equal(loggerTest.Bytes(), loggerVerify.Bytes()) {
		t.Error("failed to log the request header")
		t.Log("expected:")
		t.Log(loggerVerify.String())
		t.Log("got:")
		t.Log(loggerTest.String())
	}
}

func TestLogHeaderRequestResponse(t *testing.T) {
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	req, err := http.NewRequest("GET", "https://example.org/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "Go-http-client/1.1")

	resp := &http.Response{
		Header: http.Header{
			"Foo": []string{"Bar"},
		},
		StatusCode:    http.StatusOK,
		Status:        http.StatusText(200),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          nil,
		ContentLength: 0,
	}

	ctx := &filtertest.Context{
		FRequest:  req,
		FResponse: resp,
	}

	outputVerify := bytes.NewBuffer(nil)
	req.Body = nil
	if err := req.Write(outputVerify); err != nil {
		t.Fatal(err)
	}

	loggerVerify := bytes.NewBuffer(nil)
	log.SetOutput(loggerVerify)
	log.Println(outputVerify.String())

	loggerTest := bytes.NewBuffer(nil)
	log.SetOutput(loggerTest)

	req.Body = ioutil.NopCloser(bytes.NewBufferString("foo bar baz"))

	lh := logHeader{
		request:  true,
		response: true,
	}

	lh.Request(ctx)
	if loggerTest.Len() == 0 || !bytes.Equal(loggerTest.Bytes(), loggerVerify.Bytes()) {
		t.Error("failed to log the request header")
		t.Log("expected:")
		t.Log(loggerVerify.String())
		t.Log("got:")
		t.Log(loggerTest.String())
	}

	// response
	outputVerify = bytes.NewBuffer(nil)
	resp.Body = nil
	outputVerify.WriteString("Response for ")
	outputVerify.WriteString(req.Method)
	outputVerify.WriteString(" ")
	outputVerify.WriteString(req.URL.Path)
	outputVerify.WriteString(" ")
	outputVerify.WriteString(req.Proto)
	outputVerify.WriteString("\r\n")
	outputVerify.WriteString(resp.Status)
	outputVerify.WriteString("\r\n")
	for k, v := range resp.Header {
		outputVerify.WriteString(k)
		outputVerify.WriteString(": ")
		outputVerify.WriteString(strings.Join(v, " "))
		outputVerify.WriteString("\r\n")
	}
	outputVerify.WriteString("\r\n")

	loggerVerify = bytes.NewBuffer(nil)
	log.SetOutput(loggerVerify)
	log.Println(outputVerify.String())

	loggerTest = bytes.NewBuffer(nil)
	log.SetOutput(loggerTest)

	lh.Response(ctx)
	if loggerTest.Len() == 0 || !bytes.Equal(loggerTest.Bytes(), loggerVerify.Bytes()) {
		t.Error("failed to log the response header")
		t.Log("expected:")
		t.Log(loggerVerify.String())
		t.Log("got:")
		t.Log(loggerTest.String())
	}

}
