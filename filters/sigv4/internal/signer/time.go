package signer

import (
	"bytes"
	"fmt"
	"time"
)

// TimeFormat is the time format to be used in the X-Amz-Date header or query parameter
const TimeFormat = "20060102T150405Z"

const signingAlgorithm = "AWS4-HMAC-SHA256"

// ShortTimeFormat is the shorten time format used in the credential scope
const ShortTimeFormat = "20060102"

const AmzAlgorithmKey = "X-Amz-Algorithm"

const AmzDateKey = "X-Amz-Date"

// AmzSecurityTokenKey indicates the security token to be used with temporary credentials
const AmzSecurityTokenKey = "X-Amz-Security-Token"

type SigningTime struct {
	time.Time
	timeFormat      string
	shortTimeFormat string
}

func (m *SigningTime) TimeFormat() string {
	return m.format(&m.timeFormat, TimeFormat)
}

// ShortTimeFormat provides a time formatted of 20060102.
func (m *SigningTime) ShortTimeFormat() string {
	return m.format(&m.shortTimeFormat, ShortTimeFormat)
}

func (m *SigningTime) format(target *string, format string) string {
	if len(*target) > 0 {
		return *target
	}
	v := m.Time.Format(format)
	*target = v
	return v
}

var noEscape [256]bool

func InitializeEscape() {
	for i := 0; i < len(noEscape); i++ {
		// AWS expects every character except these to be escaped
		noEscape[i] = (i >= 'A' && i <= 'Z') ||
			(i >= 'a' && i <= 'z') ||
			(i >= '0' && i <= '9') ||
			i == '-' ||
			i == '.' ||
			i == '_' ||
			i == '~'
	}
}

// EscapePath escapes part of a URL path in Amazon style.
func EscapePath(path string, encodeSep bool) string {
	InitializeEscape() //TODO : is getting initialized every time
	var buf bytes.Buffer
	for i := 0; i < len(path); i++ {
		c := path[i]
		if noEscape[c] || (c == '/' && !encodeSep) {
			buf.WriteByte(c)
		} else {
			fmt.Fprintf(&buf, "%%%02X", c)
		}
	}
	return buf.String()
}
