package kubernetes

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

type logger struct {
	logger *log.Entry

	mu      sync.Mutex
	history map[string]struct{}
}

// newLogger creates a logger that logs each unique message once
// for the resource identified by kind, namespace and name.
// It logs nothing when disabled.
func newLogger(kind, namespace, name string, enabled bool) *logger {
	if !enabled {
		return nil
	}
	return &logger{logger: log.WithFields(log.Fields{"kind": kind, "ns": namespace, "name": name})}
}

func (l *logger) Tracef(format string, args ...any) {
	if l != nil {
		l.once(log.TraceLevel, format, args...)
	}
}

func (l *logger) Debugf(format string, args ...any) {
	if l != nil {
		l.once(log.DebugLevel, format, args...)
	}
}

func (l *logger) Infof(format string, args ...any) {
	if l != nil {
		l.once(log.InfoLevel, format, args...)
	}
}

func (l *logger) Errorf(format string, args ...any) {
	if l != nil {
		l.once(log.ErrorLevel, format, args...)
	}
}

func (l *logger) once(level log.Level, format string, args ...any) {
	if !l.logger.Logger.IsLevelEnabled(level) {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.history == nil {
		l.history = make(map[string]struct{})
	}

	msg := fmt.Sprintf(format, args...)
	key := fmt.Sprintf("%s %s", level, msg)
	if _, ok := l.history[key]; !ok {
		l.logger.Log(level, msg)
		l.history[key] = struct{}{}
	}
}
