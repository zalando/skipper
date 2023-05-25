package logging

import (
	"github.com/sirupsen/logrus"
)

// DefaultLog provides a default implementation of the Logger interface.
type DefaultLog struct {
	logger logrus.Logger
	fields map[string]interface{}
}

// Logger instances provide custom logging.
type Logger interface {

	// Log with level ERROR
	Error(...interface{})

	// Log formatted messages with level ERROR
	Errorf(string, ...interface{})

	// Log with level WARN
	Warn(...interface{})

	// Log formatted messages with level WARN
	Warnf(string, ...interface{})

	// Log with level INFO
	Info(...interface{})

	// Log formatted messages with level INFO
	Infof(string, ...interface{})

	// Log with level DEBUG
	Debug(...interface{})

	// Log formatted messages with level DEBUG
	Debugf(string, ...interface{})

	WithFields(map[string]interface{}) Logger
}

func (dl *DefaultLog) Error(a ...interface{}) { dl.logger.WithFields(dl.fields).Error(a...) }
func (dl *DefaultLog) Errorf(f string, a ...interface{}) {
	dl.logger.WithFields(dl.fields).Errorf(f, a...)
}
func (dl *DefaultLog) Warn(a ...interface{}) { dl.logger.WithFields(dl.fields).Warn(a...) }
func (dl *DefaultLog) Warnf(f string, a ...interface{}) {
	dl.logger.WithFields(dl.fields).Warnf(f, a...)
}
func (dl *DefaultLog) Info(a ...interface{}) { dl.logger.WithFields(dl.fields).Info(a...) }
func (dl *DefaultLog) Infof(f string, a ...interface{}) {
	dl.logger.WithFields(dl.fields).Infof(f, a...)
}
func (dl *DefaultLog) Debug(a ...interface{}) { dl.logger.WithFields(dl.fields).Debug(a...) }
func (dl *DefaultLog) Debugf(f string, a ...interface{}) {
	dl.logger.WithFields(dl.fields).Debugf(f, a...)
}
func (dl *DefaultLog) WithFields(fields map[string]interface{}) Logger {
	for k, v := range fields {
		dl.fields[k] = v
	}
	return dl
}
func New() *DefaultLog {
	return &DefaultLog{logger: *logrus.New()}
}
