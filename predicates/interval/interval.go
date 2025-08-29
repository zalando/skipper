/*
Package interval implements custom predicates to match routes
only during some period of time.

Package includes three predicates: Between, Before and After.
All predicates can be created using the date represented as:
  - a string in RFC3339 format (see https://golang.org/pkg/time/#pkg-constants)
  - a string in RFC3339 format without numeric timezone offset and a location name (see https://golang.org/pkg/time/#LoadLocation)
  - an int64 or float64 number corresponding to the given Unix time in seconds since January 1, 1970 UTC.
    float64 number will be converted into int64 number.

Between predicate matches only if current date is inside the specified
range of dates. Between predicate requires two dates to be constructed.
Upper boundary must be after lower boundary. Range includes the lower
boundary, but excludes the upper boundary.

Before predicate matches only if current date is before the specified
date. Only one date is required to construct the predicate.

After predicate matches only if current date is after or equal to
the specified date. Only one date is required to construct the predicate.

Examples:

	example1: Path("/zalando") && Between("2016-01-01T12:00:00+02:00", "2016-02-01T12:00:00+02:00") -> "https://www.zalando.de";
	example2: Path("/zalando") && Between(1451642400, 1454320800) -> "https://www.zalando.de";

	example3: Path("/zalando") && Before("2016-02-01T12:00:00+02:00") -> "https://www.zalando.de";
	example4: Path("/zalando") && Before(1454320800) -> "https://www.zalando.de";

	example5: Path("/zalando") && After("2016-01-01T12:00:00+02:00") -> "https://www.zalando.de";
	example6: Path("/zalando") && After(1451642400) -> "https://www.zalando.de";

	example7: Path("/zalando") && Between("2021-02-18T00:00:00", "2021-02-18T01:00:00", "Europe/Berlin") -> "https://www.zalando.de";
	example8: Path("/zalando") && Before("2021-02-18T00:00:00", "Europe/Berlin") -> "https://www.zalando.de";
	example9: Path("/zalando") && After("2021-02-18T00:00:00", "Europe/Berlin") -> "https://www.zalando.de";
*/
package interval

import (
	"net/http"
	"time"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type spec int

const (
	between spec = iota
	before
	after
)

const rfc3339nz = "2006-01-02T15:04:05" // RFC3339 without numeric timezone offset

type predicate struct {
	typ     spec
	begin   time.Time
	end     time.Time
	getTime func() time.Time
}

// NewBetween creates Between predicate.
func NewBetween() routing.PredicateSpec { return between }

// NewBefore creates Before predicate.
func NewBefore() routing.PredicateSpec { return before }

// NewAfter creates After predicate.
func NewAfter() routing.PredicateSpec { return after }

func (s spec) Name() string {
	switch s {
	case between:
		return predicates.BetweenName
	case before:
		return predicates.BeforeName
	case after:
		return predicates.AfterName
	default:
		panic("invalid interval predicate type")
	}
}

func (s spec) Create(args []interface{}) (routing.Predicate, error) {
	p := predicate{typ: s, getTime: time.Now}
	var loc *time.Location
	switch {
	case
		s == between && len(args) == 3 && parseLocation(args[2], &loc) && parseRFCnz(args[0], &p.begin, loc) && parseRFCnz(args[1], &p.end, loc) && p.begin.Before(p.end),
		s == between && len(args) == 2 && parseRFC(args[0], &p.begin) && parseRFC(args[1], &p.end) && p.begin.Before(p.end),
		s == between && len(args) == 2 && parseUnix(args[0], &p.begin) && parseUnix(args[1], &p.end) && p.begin.Before(p.end),

		s == before && len(args) == 2 && parseLocation(args[1], &loc) && parseRFCnz(args[0], &p.end, loc),
		s == before && len(args) == 1 && parseRFC(args[0], &p.end),
		s == before && len(args) == 1 && parseUnix(args[0], &p.end),

		s == after && len(args) == 2 && parseLocation(args[1], &loc) && parseRFCnz(args[0], &p.begin, loc),
		s == after && len(args) == 1 && parseRFC(args[0], &p.begin),
		s == after && len(args) == 1 && parseUnix(args[0], &p.begin):

		return &p, nil
	}
	return nil, predicates.ErrInvalidPredicateParameters
}

func parseUnix(arg interface{}, t *time.Time) bool {
	switch a := arg.(type) {
	case float64:
		*t = time.Unix(int64(a), 0)
		return true
	case int64:
		*t = time.Unix(a, 0)
		return true
	}
	return false
}

func parseRFC(arg interface{}, t *time.Time) bool {
	if s, ok := arg.(string); ok {
		tt, err := time.Parse(time.RFC3339, s)
		if err == nil {
			*t = tt
			return true
		}
	}
	return false
}

func parseRFCnz(arg interface{}, t *time.Time, loc *time.Location) bool {
	if s, ok := arg.(string); ok {
		tt, err := time.ParseInLocation(rfc3339nz, s, loc)
		if err == nil {
			*t = tt
			return true
		}
	}
	return false
}

func parseLocation(arg interface{}, loc **time.Location) bool {
	if s, ok := arg.(string); ok {
		location, err := time.LoadLocation(s)
		if err == nil {
			*loc = location
			return true
		}
	}
	return false
}

func (p *predicate) Match(r *http.Request) bool {
	now := p.getTime()

	switch p.typ {
	case between:
		return (p.begin.Before(now) || p.begin.Equal(now)) && p.end.After(now)
	case before:
		return p.end.After(now)
	case after:
		return p.begin.Before(now) || p.begin.Equal(now)
	default:
		return false
	}
}
