package logging_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/logging"
)

func TestLogger(t *testing.T) {
	log := logging.DefaultLog{}

	buf := &bytes.Buffer{}
	log.SetOutput(buf)
	log.SetLevel(logrus.DebugLevel)
	log.SetFormatter(&logrus.TextFormatter{})

	log.Error("error")
	s := buf.String()
	buf.Reset()
	if !strings.HasSuffix(s, "error\n") {
		t.Fatalf(`Failed log.Error: want suffix "error", got %q`, s)
	}

	log.Errorf("errorf: %s", "foo")
	s = strings.TrimSpace(buf.String())
	buf.Reset()
	if !strings.HasSuffix(s, `errorf: foo"`) {
		t.Fatalf(`Failed log.Errorf: want suffix "errorf: foo", got %q`, s)
	}

	log.Warn("warn")
	s = buf.String()
	buf.Reset()
	if !strings.HasSuffix(s, "warn\n") {
		t.Fatalf(`Failed log.Warn: want suffix "warn", got %q`, s)
	}

	log.Warnf("warnf: %s", "foo")
	s = strings.TrimSpace(buf.String())
	buf.Reset()
	if !strings.HasSuffix(s, `warnf: foo"`) {
		t.Fatalf(`Failed log.Warnf: want suffix "warnf: foo", got %q`, s)
	}

	log.Info("info")
	s = buf.String()
	buf.Reset()
	if !strings.HasSuffix(s, "info\n") {
		t.Fatalf(`Failed log.Info: want suffix "info", got %q`, s)
	}

	log.Infof("infof: %s", "foo")
	s = strings.TrimSpace(buf.String())
	buf.Reset()
	if !strings.HasSuffix(s, `infof: foo"`) {
		t.Fatalf(`Failed log.Infof: want suffix "infof: foo", got %q`, s)
	}

	log.Debug("debug")
	s = buf.String()
	buf.Reset()
	if !strings.HasSuffix(s, "debug\n") {
		t.Fatalf(`Failed log.Debug: want suffix "debug", got %q`, s)
	}

	log.Debugf("debugf: %s", "foo")
	s = strings.TrimSpace(buf.String())
	buf.Reset()
	if !strings.HasSuffix(s, `debugf: foo"`) {
		t.Fatalf(`Failed log.Debugf: want suffix "debugf: foo", got %q`, s)
	}

}
