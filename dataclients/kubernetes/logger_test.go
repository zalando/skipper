package kubernetes

import (
	"bytes"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func captureLog(t *testing.T, level log.Level) *bytes.Buffer {
	oldOut := log.StandardLogger().Out
	var out bytes.Buffer
	log.SetOutput(&out)

	oldLevel := log.GetLevel()
	log.SetLevel(level)

	t.Cleanup(func() {
		log.SetOutput(oldOut)
		log.SetLevel(oldLevel)
	})
	return &out
}

func TestLogger(t *testing.T) {
	t.Run("test disabled", func(t *testing.T) {
		out := captureLog(t, log.DebugLevel)

		l := newLogger("ingress", "foo", "bar", false)
		l.Debugf("test message")
		l.Infof("test message")
		l.Errorf("test message")

		assert.Equal(t, 0, strings.Count(out.String(), "test message"))
	})

	t.Run("test level", func(t *testing.T) {
		out := captureLog(t, log.ErrorLevel)

		l := newLogger("ingress", "foo", "bar", true)
		l.Debugf("test message")
		l.Infof("test message")
		l.Errorf("test message")

		assert.Equal(t, 0, strings.Count(out.String(), `level=debug`))
		assert.Equal(t, 0, strings.Count(out.String(), `level=info`))
		assert.Equal(t, 1, strings.Count(out.String(), `level=error`))
	})

	t.Run("test logs once per level", func(t *testing.T) {
		out := captureLog(t, log.DebugLevel)

		l := newLogger("ingress", "foo", "bar", true)

		msg1 := "test message1 %d %s"
		args1 := []any{1, "qux"}
		for range 3 {
			l.Debugf(msg1, args1...)
			l.Infof(msg1, args1...)
			l.Errorf(msg1, args1...)
		}

		msg2 := "test message2 %d %s"
		args2 := []any{2, "quux"}
		for range 3 {
			l.Debugf(msg2, args2...)
			l.Infof(msg2, args2...)
			l.Errorf(msg2, args2...)
		}

		t.Logf("log output: %s", out.String())

		assert.Equal(t, 1, strings.Count(out.String(), `level=debug msg="test message1 1 qux" kind=ingress name=bar ns=foo`))
		assert.Equal(t, 1, strings.Count(out.String(), `level=info msg="test message1 1 qux" kind=ingress name=bar ns=foo`))
		assert.Equal(t, 1, strings.Count(out.String(), `level=error msg="test message1 1 qux" kind=ingress name=bar ns=foo`))
		assert.Equal(t, 1, strings.Count(out.String(), `level=debug msg="test message2 2 quux" kind=ingress name=bar ns=foo`))
		assert.Equal(t, 1, strings.Count(out.String(), `level=info msg="test message2 2 quux" kind=ingress name=bar ns=foo`))
		assert.Equal(t, 1, strings.Count(out.String(), `level=error msg="test message2 2 quux" kind=ingress name=bar ns=foo`))
	})
}
