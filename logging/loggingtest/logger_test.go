package loggingtest_test

import (
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
	lt.Info("info")
	if n := lt.Count("info"); n != 0 {
		t.Fatalf(`Failed to get 0 times "info", got %d`, n)
	}

	lt.Unmute()
	lt.Info("info")
	if n := lt.Count("info"); n != 1 {
		t.Fatalf(`Failed to get 1 times "info", got %d`, n)
	}
}
