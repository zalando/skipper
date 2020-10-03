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

// Init options for logging.
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

	// Output for the access log entries, when nil, os.Stderr is
	// used.
	AccessLogOutput io.Writer

	// When set, log in JSON format is used
	AccessLogJSONEnabled bool

	// AccessLogStripQuery, when set, causes the query strings stripped
	// from the request URI in the access logs.
	AccessLogStripQuery bool

	// LogrusJsonFormatter is passed along to to the underlying Logrus logging if JSON logging is enabled.
	// To enable structured logging, use the ApplicationLogJSONEnabled and AccessLogJSONEnabled settings.
	// In the case of access logs, the timestamp format will be overwritten and timestamps will be disabled.
	LogrusJsonFormatter logrus.JSONFormatter
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
		logrus.SetFormatter(&o.LogrusJsonFormatter)
	} else if o.ApplicationLogPrefix != "" {
		logrus.SetFormatter(&prefixFormatter{o.ApplicationLogPrefix, logrus.StandardLogger().Formatter})
	}

	if o.ApplicationLogOutput != nil {
		logrus.SetOutput(o.ApplicationLogOutput)
	}
}

func initAccessLog(o Options) {
	l := logrus.New()
	if o.AccessLogJSONEnabled {
		o.LogrusJsonFormatter.TimestampFormat = dateFormat
		o.LogrusJsonFormatter.DisableTimestamp = true
		l.Formatter = &o.LogrusJsonFormatter
	} else {
		l.Formatter = &accessLogFormatter{accessLogFormat}
	}
	l.Out = o.AccessLogOutput
	l.Level = logrus.InfoLevel
	accessLog = l
	stripQuery = o.AccessLogStripQuery
}

// Initializes logging.
func Init(o Options) {
	initApplicationLog(o)

	if o.AccessLogOutput == nil {
		o.AccessLogOutput = os.Stderr
	}

	initAccessLog(o)
}
