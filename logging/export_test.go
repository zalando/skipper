package logging

import (
	"io"

	"github.com/sirupsen/logrus"
)

func (dl *DefaultLog) SetOutput(out io.Writer)                   { logrus.SetOutput(out) }
func (dl *DefaultLog) SetLevel(level logrus.Level)               { logrus.SetLevel(level) }
func (dl *DefaultLog) SetFormatter(format *logrus.TextFormatter) { logrus.SetFormatter(format) }
