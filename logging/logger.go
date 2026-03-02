package logging

import "github.com/sirupsen/logrus"

// DefaultLog provides a default implementation of the Logger interface.
type DefaultLog struct{}

// Logger instances provide custom logging.
type Logger interface {

	// Log with level ERROR
	Error(...any)

	// Log formatted messages with level ERROR
	Errorf(string, ...any)

	// Log with level WARN
	Warn(...any)

	// Log formatted messages with level WARN
	Warnf(string, ...any)

	// Log with level INFO
	Info(...any)

	// Log formatted messages with level INFO
	Infof(string, ...any)

	// Log with level DEBUG
	Debug(...any)

	// Log formatted messages with level DEBUG
	Debugf(string, ...any)
}

func (dl *DefaultLog) Error(a ...any)            { logrus.Error(a...) }
func (dl *DefaultLog) Errorf(f string, a ...any) { logrus.Errorf(f, a...) }
func (dl *DefaultLog) Warn(a ...any)             { logrus.Warn(a...) }
func (dl *DefaultLog) Warnf(f string, a ...any)  { logrus.Warnf(f, a...) }
func (dl *DefaultLog) Info(a ...any)             { logrus.Info(a...) }
func (dl *DefaultLog) Infof(f string, a ...any)  { logrus.Infof(f, a...) }
func (dl *DefaultLog) Debug(a ...any)            { logrus.Debug(a...) }
func (dl *DefaultLog) Debugf(f string, a ...any) { logrus.Debugf(f, a...) }
