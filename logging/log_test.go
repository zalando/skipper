package logging

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
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
	got := buf.String()
	if !strings.HasPrefix(got, "[TEST_PREFIX]") || !strings.Contains(got, "Hello, world!") {
		t.Error("failed to use custom prefix")
	}
}

func TestCustomOutputForAccessLog(t *testing.T) {
	var buf bytes.Buffer
	Init(Options{AccessLogOutput: &buf})
	LogAccess(&AccessEntry{StatusCode: http.StatusTeapot}, nil)
	if !strings.Contains(buf.String(), strconv.Itoa(http.StatusTeapot)) {
		t.Error("failed to use custom access log output")
	}
}
