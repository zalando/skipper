package loggingtest_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/logging/loggingtest"
)

func TestLoggingTest(t *testing.T) {
	lt := loggingtest.New()
	defer lt.Close()

	lt.Debug("debug")
	lt.Debugf("debugf: %s", "foo")
	lt.Info("info")
	lt.Infof("infof: %s", "foo")
	lt.Warn("warn")
	lt.Warnf("warnf: %s", "foo")
	lt.Error("error")
	lt.Errorf("errorf: %s", "foo")
	for _, s := range []string{"debug", "debugf: foo", "info", "infof: foo",
		"warn", "warnf: foo", "error", "errorf: foo"} {
		if err := lt.WaitFor(s, time.Second); err != nil {
			t.Fatalf("Failed to get %q: %v", s, err)
		}
	}

	if n := lt.Count("info"); n != 2 {
		t.Fatalf(`Failed to get two times "info", got %d`, n)
	}

	lt.Reset()
	if err := lt.WaitForN("foo", 2, time.Millisecond); err != loggingtest.ErrWaitTimeout {
		t.Fatalf("Failed to get err want: %v, got: %v", loggingtest.ErrWaitTimeout, err)
	}

	lt.Mute()
	out, err := captureOutput(func() { lt.Info("bar") })
	if err != nil {
		t.Fatalf("Failed to capture log output: %v", err)
	}
	if strings.Contains(out, "bar") {
		t.Fatal("Failed to mute log output")
	}
	lt.Unmute()

	lt.Info("info")
	if n := lt.Count("info"); n != 1 {
		t.Fatalf(`Failed to get 1 times "info", got %d`, n)
	}
}

func captureOutput(f func()) (string, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}

	stdout := os.Stdout
	stderr := os.Stderr
	defer func() {
		os.Stdout = stdout
		os.Stderr = stderr
	}()

	os.Stdout = writer
	os.Stderr = writer

	outChan := make(chan string)
	var wg sync.WaitGroup

	wg.Go(func() {
		var buf bytes.Buffer
		io.Copy(&buf, reader)
		outChan <- buf.String()
	})
	f()

	writer.Close()
	return <-outChan, nil
}
