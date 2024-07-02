package awssigner

import (
	"time"
)

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

func isSameDay(x, y time.Time) bool {
	xYear, xMonth, xDay := x.Date()
	yYear, yMonth, yDay := y.Date()

	if xYear != yYear {
		return false
	}

	if xMonth != yMonth {
		return false
	}

	return xDay == yDay
}

func NewSigningTime(t time.Time) SigningTime {
	return SigningTime{
		Time: t,
	}
}
