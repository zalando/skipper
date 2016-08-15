package logging

import "github.com/Sirupsen/logrus"

// DefaultLog provides a default implementation of the Logger interface.
type DefaultLog struct{}

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
}

func (dl *DefaultLog) Error(a ...interface{})            { logrus.Error(a...) }
func (dl *DefaultLog) Errorf(f string, a ...interface{}) { logrus.Errorf(f, a...) }
func (dl *DefaultLog) Warn(a ...interface{})             { logrus.Warn(a...) }
func (dl *DefaultLog) Warnf(f string, a ...interface{})  { logrus.Warnf(f, a...) }
func (dl *DefaultLog) Info(a ...interface{})             { logrus.Info(a...) }
func (dl *DefaultLog) Infof(f string, a ...interface{})  { logrus.Infof(f, a...) }
func (dl *DefaultLog) Debug(a ...interface{})            { logrus.Debug(a...) }
func (dl *DefaultLog) Debugf(f string, a ...interface{}) { logrus.Debugf(f, a...) }
