package logging

import (
	"github.com/Sirupsen/logrus"
	"io"
)

type prefixFormatter struct {
	prefix    string
	formatter logrus.Formatter
}

type Options struct {
	ApplicationLogPrefix string
	ApplicationLogOutput io.Writer
	AccessLogOutput      io.Writer
}

var accessLog *logrus.Logger

func (f *prefixFormatter) Format(e *logrus.Entry) ([]byte, error) {
	b, err := f.formatter.Format(e)
	if err != nil {
		return nil, err
	}

	return append([]byte(f.prefix), b...), nil
}

func initApplicationLog(prefix string, output io.Writer) {
	if prefix != "" {
		logrus.SetFormatter(&prefixFormatter{
			prefix, logrus.StandardLogger().Formatter})
	}

	if output != nil {
		logrus.SetOutput(output)
	}
}

func initAccessLog(output io.Writer) {
	l := logrus.New()
	l.Formatter = &accessLogFormatter{accessLogFormat}
	l.Out = output
	l.Level = logrus.InfoLevel
	accessLog = l
}

func Init(o Options) {
	if o.ApplicationLogPrefix != "" || o.ApplicationLogOutput != nil {
		initApplicationLog(o.ApplicationLogPrefix, o.ApplicationLogOutput)
	}

	if o.AccessLogOutput != nil {
		initAccessLog(o.AccessLogOutput)
	}
}

func ApplicationLog() *logrus.Logger { return logrus.StandardLogger() }
func AccessLog() *logrus.Logger      { return accessLog }
