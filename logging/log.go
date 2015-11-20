package logging

import (
	"github.com/Sirupsen/logrus"
	"io"
	"os"
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

	// Output for the access log entries, when nil, os.Stderr is
	// used.
	AccessLogOutput io.Writer

	// When set, no access log is printed.
	AccessLogDisabled bool
}

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

// Initializes logging.
func Init(o Options) {
	if o.ApplicationLogPrefix != "" || o.ApplicationLogOutput != nil {
		initApplicationLog(o.ApplicationLogPrefix, o.ApplicationLogOutput)
	}

	if !o.AccessLogDisabled {
		if o.AccessLogOutput == nil {
			o.AccessLogOutput = os.Stderr
		}

		initAccessLog(o.AccessLogOutput)
	}
}
