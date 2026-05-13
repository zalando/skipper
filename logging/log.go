package logging

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

type prefixFormatter struct {
	prefix    string
	formatter logrus.Formatter
}

// Options for logging.
type Options struct {

	// Prefix for application log entries. Primarily used to be
	// able to select between access log and application log
	// entries.
	ApplicationLogPrefix string

	// Output for the application log entries, when nil,
	// os.Stderr is used.
	ApplicationLogOutput io.Writer

	// When set, log in JSON format is used
	ApplicationLogJSONEnabled bool

	// ApplicationLogJsonFormatter, when set and JSON logging is enabled, is passed along to to the underlying
	// Logrus logger for application logs. To enable structured logging, use ApplicationLogJSONEnabled.
	ApplicationLogJsonFormatter *logrus.JSONFormatter

	// Output for the access log entries, when nil, os.Stderr is
	// used.
	AccessLogOutput io.Writer

	// When set, log in JSON format is used
	AccessLogJSONEnabled bool

	// AccessLogStripQuery, when set, causes the query strings stripped
	// from the request URI in the access logs.
	AccessLogStripQuery bool

	// AccessLogJsonFormatter, when set and JSON logging is enabled, is passed along to to the underlying
	// Logrus logger for access logs. To enable structured logging, use AccessLogJSONEnabled.
	// Deprecated: use [AccessLogFormatter].
	AccessLogJsonFormatter *logrus.JSONFormatter

	// AccessLogFormatter, when set is passed along to the underlying Logrus logger for access logs.
	AccessLogFormatter logrus.Formatter
}

func (f *prefixFormatter) Format(e *logrus.Entry) ([]byte, error) {
	b, err := f.formatter.Format(e)
	if err != nil {
		return nil, err
	}

	return append([]byte(f.prefix), b...), nil
}

func initApplicationLog(o Options) {
	if o.ApplicationLogJSONEnabled {
		if o.ApplicationLogJsonFormatter != nil {
			logrus.SetFormatter(o.ApplicationLogJsonFormatter)
		} else {
			logrus.SetFormatter(&logrus.JSONFormatter{})
		}
	} else if o.ApplicationLogPrefix != "" {
		logrus.SetFormatter(&prefixFormatter{o.ApplicationLogPrefix, logrus.StandardLogger().Formatter})
	}

	if o.ApplicationLogOutput != nil {
		logrus.SetOutput(o.ApplicationLogOutput)
	}
}

func createAccessLog(o Options) *AccessLogger {
	l := logrus.New()
	if o.AccessLogFormatter != nil {
		l.Formatter = o.AccessLogFormatter
	} else if o.AccessLogJSONEnabled {
		if o.AccessLogJsonFormatter != nil {
			l.Formatter = o.AccessLogJsonFormatter
		} else {
			l.Formatter = &logrus.JSONFormatter{TimestampFormat: dateFormat, DisableTimestamp: true}
		}
	} else {
		l.Formatter = &accessLogFormatter{accessLogFormat}
	}
	l.Out = o.AccessLogOutput
	l.Level = logrus.InfoLevel

	return &AccessLogger{
		stripQuery: o.AccessLogStripQuery,
		log:        l,
	}
}

func NewAccessLogger(o Options) *AccessLogger {
	initApplicationLog(o)

	if o.AccessLogOutput == nil {
		o.AccessLogOutput = os.Stderr
	}
	return createAccessLog(o)
}
