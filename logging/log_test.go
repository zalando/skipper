package logging

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

func TestCustomOutputForApplicationLog(t *testing.T) {
	var buf bytes.Buffer
	Init(Options{ApplicationLogOutput: &buf})
	msg := "Hello, world!"
	log.Info(msg)
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
	LogAccess(&AccessEntry{StatusCode: http.StatusTeapot}, nil, nil)
	if !strings.Contains(buf.String(), strconv.Itoa(http.StatusTeapot)) {
		t.Error("failed to use custom access log output")
	}
}

func TestApplicationLogJSONEnabled(t *testing.T) {
	var buf bytes.Buffer
	Init(Options{ApplicationLogOutput: &buf, ApplicationLogJSONEnabled: true})
	msg := "Hello, world!"
	log.Info(msg)

	parsed := make(map[string]interface{})
	err := json.Unmarshal(buf.Bytes(), &parsed)
	if err != nil {
		t.Errorf("failed to parse json log: %v", err)
	}

	if got := parsed["level"]; got != "info" {
		t.Errorf("invalid level, expected: info, got: %v", got)
	}

	if got := parsed["msg"]; got != msg {
		t.Errorf("invalid msg, expected: %s, got: %v", msg, got)
	}

	if got, ok := parsed["time"]; ok {
		_, err := time.Parse(time.RFC3339, got.(string))
		if err != nil {
			t.Errorf("failed to parse time: %v", err)
		}
	} else {
		t.Error("time is missing")
	}

	if len(parsed) != 3 {
		t.Errorf("unexpected field count")
	}
}

func TestApplicationLogJSONWithCustomFormatter(t *testing.T) {
	var buf bytes.Buffer
	Init(Options{
		ApplicationLogOutput:      &buf,
		ApplicationLogJSONEnabled: true,
		ApplicationLogJsonFormatter: &log.JSONFormatter{
			FieldMap: log.FieldMap{
				log.FieldKeyLevel: "my_level",
				log.FieldKeyMsg:   "my_message",
				log.FieldKeyTime:  "my_time",
			},
		}})

	msg := "Hello, customized world!"
	log.Info(msg)

	parsed := make(map[string]interface{})
	err := json.Unmarshal(buf.Bytes(), &parsed)
	if err != nil {
		t.Errorf("failed to parse json log: %v", err)
	}

	if got := parsed["my_level"]; got != "info" {
		t.Errorf("invalid level, expected: info, got: %v", got)
	}

	if got := parsed["my_message"]; got != msg {
		t.Errorf("invalid msg, expected: %s, got: %v", msg, got)
	}

	if got, ok := parsed["my_time"]; ok {
		_, err := time.Parse(time.RFC3339, got.(string))
		if err != nil {
			t.Errorf("failed to parse time: %v", err)
		}
	} else {
		t.Error("time is missing")
	}

	if len(parsed) != 3 {
		t.Errorf("unexpected field count")
	}
}

type customFormatter struct {
	innerFormatter *log.JSONFormatter
}

func (f *customFormatter) Format(entry *log.Entry) ([]byte, error) {
	originalBytes, err := f.innerFormatter.Format(entry)
	if err != nil {
		return nil, err
	}
	newBytes := bytes.NewBuffer(originalBytes)
	newBytes.WriteString(" - Custom Suffix")
	return newBytes.Bytes(), nil
}

func TestAccessLogFormatterTakesPrecedence(t *testing.T) {
	var buf bytes.Buffer
	f := &customFormatter{innerFormatter: &log.JSONFormatter{}}
	Init(Options{AccessLogOutput: &buf, AccessLogFormatter: f})
	LogAccess(&AccessEntry{StatusCode: http.StatusTeapot}, nil)
	s := buf.String()
	if !strings.Contains(s, " - Custom Suffix") {
		t.Error("failed to use custom access log output")
	}
}
