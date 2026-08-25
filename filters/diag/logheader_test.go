package diag

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestCreateFilterLogHeader(t *testing.T) {
	lgh := logHeader{}
	f, err := lgh.CreateFilter([]any{"request", "response"})
	if err != nil {
		t.Fatal(err)
	}
	lgh = f.(logHeader)
	if !(lgh.request && lgh.response) {
		t.Errorf("Failed to set members: %v %v", lgh.request, lgh.response)
	}
}
func TestCreateFilterLogHeaderWrongInput(t *testing.T) {
	lgh := logHeader{}
	_, err := lgh.CreateFilter([]any{5})
	if err == nil {
		t.Fatal("Failed to get expected error 5 is no string")
	}
}

func TestLogHeader(t *testing.T) {
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	req, err := http.NewRequest("GET", "https://example.org/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "Go-http-client/1.1")

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

	req.Body = io.NopCloser(bytes.NewBufferString("foo bar baz"))

	lh, err := (logHeader{}).CreateFilter(nil)
	if err != nil {
		t.Fatal(err)
	}

	lh.Request(ctx)
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

	req.Body = io.NopCloser(bytes.NewBufferString("foo bar baz"))

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

func TestLogHeaderAuthorizationRequestResponse(t *testing.T) {
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	req, err := http.NewRequest("GET", "https://example.org/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "secret")

	// req.Header is a map so for non flaky tests we need to drop the default header from expected output
	req.Header.Del("User-Agent")

	resp := &http.Response{
		Header: http.Header{
			"Authorization": []string{"secret"},
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
	s := outputVerify.String()
	s = strings.ReplaceAll(s, "secret", "TRUNCATED")
	// req.Header is a map so for non flaky tests we need to drop the default header from expected output
	s = strings.ReplaceAll(s, "User-Agent: Go-http-client/1.1\r\n", "")
	log.Println(s)

	loggerTest := bytes.NewBuffer(nil)
	log.SetOutput(loggerTest)

	req.Body = io.NopCloser(bytes.NewBufferString("foo bar baz"))

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
		if k == "Authorization" {
			v = []string{"TRUNCATED"}
		}
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
