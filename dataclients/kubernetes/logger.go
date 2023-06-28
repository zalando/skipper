package kubernetes

import log "github.com/sirupsen/logrus"

type logger struct {
	logger *log.Entry
}

func newLogger(kind, namespace, name string, enabled bool) *logger {
	if !enabled {
		return nil
	}
	return &logger{log.WithFields(log.Fields{"kind": kind, "ns": namespace, "name": name})}
}

func (l *logger) Debugf(format string, args ...any) {
	if l != nil {
		l.logger.Debugf(format, args...)
	}
}

func (l *logger) Infof(format string, args ...any) {
	if l != nil {
		l.logger.Infof(format, args...)
	}
}

func (l *logger) Errorf(format string, args ...any) {
	if l != nil {
		l.logger.Errorf(format, args...)
	}
}
