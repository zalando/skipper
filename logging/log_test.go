package logging

import (
	"bytes"
	log "github.com/Sirupsen/logrus"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestCustomOutputForApplicationLog(t *testing.T) {
	var buf bytes.Buffer
	Init(Options{ApplicationLogOutput: &buf})
	msg := "Hello, world!"
	log.Infof(msg)
	if !strings.Contains(buf.String(), msg) {
		t.Error("failed to use custom output")
	}
}

func TestCustomPrefixForApplicationLog(t *testing.T) {
	var buf bytes.Buffer
	prefix := "[TEST_PREFIX]"
	Init(Options{
		ApplicationLogOutput: &buf,
		ApplicationLogPrefix: prefix})
	log.Infof("Hello, world!")
	if strings.Index(buf.String(), prefix) != 0 {
		t.Error("failed to use custom prefix")
	}
}

func TestCustomOutputForAccessLog(t *testing.T) {
	var buf bytes.Buffer
	Init(Options{AccessLogOutput: &buf})
	LogAccess(&AccessEntry{StatusCode: http.StatusTeapot})
	if !strings.Contains(buf.String(), strconv.Itoa(http.StatusTeapot)) {
		t.Error("failed to use custom access log output")
	}
}

func TestDisableAccessLog(t *testing.T) {
	var buf bytes.Buffer
	Init(Options{
		AccessLogOutput:   &buf,
		AccessLogDisabled: true})
	LogAccess(&AccessEntry{StatusCode: http.StatusTeapot})
	if buf.Len() != 0 {
		t.Error("failed to disable access log")
	}
}
