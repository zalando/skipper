package interval

import (
	"net/http"
	"testing"
	"time"
)

func TestCreateBetween(t *testing.T) {
	cases := []struct {
		msg  string
		args []interface{}
		err  bool
	}{
		{
			"nil arguments",
			nil,
			true,
		},
		{
			"wrong number of arguments",
			[]interface{}{"2016-01-01T12:00:00+07:00"},
			true,
		},
		{
			"wrong number of arguments",
			[]interface{}{"2016-01-01T12:00:00+07:00", "2016-02-01T12:00:00+07:00", "2016-03-01T12:00:00+07:00"},
			true,
		},
		{
			"first argument is not a string",
			[]interface{}{'1', "2016-01-01T12:00:00+07:00"},
			true,
		},
		{
			"second argument is not a string",
			[]interface{}{"2016-01-01T12:00:00+07:00", '1'},
			true,
		},
		{
			"mixed first unix and second string",
			[]interface{}{time.Date(2021, 2, 18, 0, 0, 0, 0, time.UTC).Unix(), "2021-02-18T01:00:00Z"},
			true,
		},
		{
			"mixed first string and second unix",
			[]interface{}{"2021-02-18T01:00:00Z", time.Date(2021, 2, 18, 2, 0, 0, 0, time.UTC).Unix()},
			true,
		},
		{
			"begin date is after end date",
			[]interface{}{"2016-02-01T12:00:00+07:00", "2016-01-01T12:00:00+07:00"},
			true,
		},
		{
			"begin date is the same as end date",
			[]interface{}{"2016-02-01T12:00:00+07:00", "2016-02-01T12:00:00+07:00"},
			true,
		},
		{
			"valid interval with time zone",
			[]interface{}{"2016-01-01T12:00:00+07:00", "2016-02-01T12:00:00+07:00"},
			false,
		},
		{
			"valid interval in UTC",
			[]interface{}{"2016-01-01T12:00:00Z", "2016-02-01T12:00:00Z"},
			false,
		},
		{
			"valid interval using Unix time",
			[]interface{}{float64(1451649600), float64(1454328000)},
			false,
		},
		{
			"valid interval, invalid location",
			[]interface{}{"2021-02-18T00:00:00", "2021-02-18T01:00:00", "unknown location"},
			true,
		},
		{
			"begin after end in valid location",
			[]interface{}{"2021-02-18T02:00:00", "2021-02-18T01:00:00", "Europe/Berlin"},
			true,
		},
		{
			"valid interval in valid location",
			[]interface{}{"2021-02-18T00:00:00", "2021-02-18T01:00:00", "Europe/Berlin"},
			false,
		},
		{
			"unsupported timezone offset in location",
			[]interface{}{"2021-02-18T00:00:00+01:00", "2021-02-18T01:00:00", "Europe/Berlin"},
			true,
		},
		{
			"unsupported unix in location",
			[]interface{}{int64(1613603624), int64(1613603625), "Europe/Berlin"},
			true,
		},
	}

	for _, c := range cases {
		_, err := NewBetween().Create(c.args)

		if err == nil && c.err || err != nil && !c.err {
			t.Errorf("%q: Is error case - %t; Error - %v", c.msg, c.err, err)
		}
	}
}

func TestCreateBefore(t *testing.T) {
	cases := []struct {
		msg  string
		args []interface{}
		err  bool
	}{
		{
			"nil arguments",
			nil,
			true,
		},
		{
			"wrong number of arguments",
			[]interface{}{"2016-01-01T12:00:00+07:00", "2016-02-01T12:00:00+07:00"},
			true,
		},
		{
			"argument is not a string",
			[]interface{}{1},
			true,
		},
		{
			"valid string argument",
			[]interface{}{"2016-01-01T12:00:00+07:00"},
			false,
		},
		{
			"valid float argument",
			[]interface{}{float64(1451624400)},
			false,
		},
		{
			"valid timestamp, invalid location",
			[]interface{}{"2021-02-18T00:00:00", "unknown location"},
			true,
		},
		{
			"valid timestamp in valid location",
			[]interface{}{"2021-02-18T00:00:00", "Europe/Berlin"},
			false,
		},
		{
			"unsupported timezone offset in location",
			[]interface{}{"2021-02-18T00:00:00+01:00", "Europe/Berlin"},
			true,
		},
		{
			"unsupported unix in location",
			[]interface{}{int64(1613603624), "Europe/Berlin"},
			true,
		},
	}

	for _, c := range cases {
		_, err := NewBefore().Create(c.args)

		if err == nil && c.err || err != nil && !c.err {
			t.Errorf("%q: Is error case - %t; Error - %v", c.msg, c.err, err)
		}
	}
}

func TestCreateAfter(t *testing.T) {
	cases := []struct {
		msg  string
		args []interface{}
		err  bool
	}{
		{
			"nil arguments",
			nil,
			true,
		},
		{
			"wrong number of arguments",
			[]interface{}{"2016-01-01T12:00:00+07:00", "2016-02-01T12:00:00+07:00"},
			true,
		},
		{
			"argument is not a string",
			[]interface{}{1},
			true,
		},
		{
			"valid string argument",
			[]interface{}{"2016-01-01T12:00:00+07:00"},
			false,
		},
		{
			"valid float argument",
			[]interface{}{float64(1451624400)},
			false,
		},
		{
			"valid timestamp, invalid location",
			[]interface{}{"2021-02-18T00:00:00", "unknown location"},
			true,
		},
		{
			"valid timestamp in valid location",
			[]interface{}{"2021-02-18T00:00:00", "Europe/Berlin"},
			false,
		},
		{
			"unsupported timezone offset in location",
			[]interface{}{"2021-02-18T00:00:00+01:00", "Europe/Berlin"},
			true,
		},
		{
			"unsupported unix in location",
			[]interface{}{int64(1613603624), "Europe/Berlin"},
			true,
		},
	}

	for _, c := range cases {
		_, err := NewAfter().Create(c.args)

		if err == nil && c.err || err != nil && !c.err {
			t.Errorf("%q: Is error case - %t; Error - %v", c.msg, c.err, err)
		}
	}
}

func TestMatchBetween(t *testing.T) {
	request := &http.Request{}

	cases := []struct {
		msg     string
		args    []interface{}
		getTime func() time.Time
		matches bool
	}{
		{
			"time inside the interval defined as strings",
			[]interface{}{time.Now().Add(-1 * time.Hour).Format(time.RFC3339), time.Now().Add(time.Hour).Format(time.RFC3339)},
			nil,
			true,
		},
		{
			"time inside the interval defined is floats",
			[]interface{}{float64(time.Now().Add(-1 * time.Hour).Unix()), float64(time.Now().Add(time.Hour).Unix())},
			nil,
			true,
		},
		{
			"time is equal to begin value",
			[]interface{}{"2016-01-01T12:00:00Z", "2016-02-01T12:00:00Z"},
			func() time.Time {
				return time.Date(2016, 1, 1, 12, 0, 0, 0, time.UTC)
			},
			true,
		},
		{
			"time is equal to end value",
			[]interface{}{"2016-01-01T12:00:00Z", "2016-02-01T12:00:00Z"},
			func() time.Time {
				return time.Date(2016, 2, 1, 12, 0, 0, 0, time.UTC)
			},
			false,
		},
		{
			"time before begin value",
			[]interface{}{time.Now().Add(time.Hour).Format(time.RFC3339), time.Now().Add(2 * time.Hour).Format(time.RFC3339)},
			nil,
			false,
		},
		{
			"time after end value",
			[]interface{}{time.Now().Add(-2 * time.Hour).Format(time.RFC3339), time.Now().Add(-1 * time.Hour).Format(time.RFC3339)},
			nil,
			false,
		},
		{
			"time inside the interval in location",
			[]interface{}{"2021-02-18T00:00:00", "2021-02-18T01:00:00", "Europe/Berlin"},
			func() time.Time {
				t, _ := time.Parse(time.RFC3339, "2021-02-17T23:01:00Z")
				return t
			},
			true,
		},
		{
			"time inside the interval in UTC location",
			[]interface{}{"2021-02-18T00:00:00", "2021-02-18T01:00:00", "UTC"},
			func() time.Time {
				t, _ := time.Parse(time.RFC3339, "2021-02-18T00:01:00Z")
				return t
			},
			true,
		},
		{
			"time before the interval in location",
			[]interface{}{"2021-02-18T00:00:00", "2021-02-18T01:00:00", "Europe/Berlin"},
			func() time.Time {
				t, _ := time.Parse(time.RFC3339, "2021-02-17T22:59:00Z")
				return t
			},
			false,
		},
		{
			"time after the interval in location",
			[]interface{}{"2021-02-18T00:00:00", "2021-02-18T01:00:00", "Europe/Berlin"},
			func() time.Time {
				t, _ := time.Parse(time.RFC3339, "2021-02-18T00:01:00Z")
				return t
			},
			false,
		},
	}

	for _, c := range cases {
		p, err := NewBetween().Create(c.args)
		if err != nil {
			t.Errorf("Failed to create predicate: %q", err)
		} else {
			betweenPredicate := p.(*predicate)
			if c.getTime != nil {
				betweenPredicate.getTime = c.getTime
			}

			matches := betweenPredicate.Match(request)

			if matches != c.matches {
				t.Errorf("%q: Expected result - %t; Actual result - %t", c.msg, c.matches, matches)
			}
		}
	}
}

func TestMatchBefore(t *testing.T) {
	request := &http.Request{}

	cases := []struct {
		msg     string
		args    []interface{}
		getTime func() time.Time
		matches bool
	}{
		{
			"time before the boundary value defined as string",
			[]interface{}{time.Now().Add(time.Hour).Format(time.RFC3339)},
			nil,
			true,
		},
		{
			"time before the boundary value defined as float",
			[]interface{}{float64(time.Now().Add(time.Hour).Unix())},
			nil,
			true,
		},
		{
			"time is equal to boundary value",
			[]interface{}{"2016-01-01T12:00:00Z"},
			func() time.Time {
				return time.Date(2016, 1, 1, 12, 0, 0, 0, time.UTC)
			},
			false,
		},
		{
			"time after boundary value",
			[]interface{}{time.Now().Add(-1 * time.Hour).Format(time.RFC3339)},
			nil,
			false,
		},
		{
			"time before the boundary in location",
			[]interface{}{"2021-02-18T01:00:00", "Europe/Berlin"},
			func() time.Time {
				t, _ := time.Parse(time.RFC3339, "2021-02-17T23:59:00Z")
				return t
			},
			true,
		},
		{
			"time after the boundary in location",
			[]interface{}{"2021-02-18T01:00:00", "Europe/Berlin"},
			func() time.Time {
				t, _ := time.Parse(time.RFC3339, "2021-02-18T00:01:00Z")
				return t
			},
			false,
		},
	}

	for _, c := range cases {
		p, err := NewBefore().Create(c.args)
		if err != nil {
			t.Errorf("Failed to create predicate: %q", err)
		} else {
			beforePredicate := p.(*predicate)
			if c.getTime != nil {
				beforePredicate.getTime = c.getTime
			}

			matches := beforePredicate.Match(request)

			if matches != c.matches {
				t.Errorf("%q: Expected result - %t; Actual result - %t", c.msg, c.matches, matches)
			}
		}
	}
}

func TestMatchAfter(t *testing.T) {
	request := &http.Request{}

	cases := []struct {
		msg     string
		args    []interface{}
		getTime func() time.Time
		matches bool
	}{
		{
			"time before the boundary value",
			[]interface{}{time.Now().Add(time.Hour).Format(time.RFC3339)},
			nil,
			false,
		},
		{
			"time is equal to boundary value",
			[]interface{}{"2016-01-01T12:00:00Z"},
			func() time.Time {
				return time.Date(2016, 1, 1, 12, 0, 0, 0, time.UTC)
			},
			true,
		},
		{
			"time after boundary value defined as string",
			[]interface{}{time.Now().Add(-1 * time.Hour).Format(time.RFC3339)},
			nil,
			true,
		},
		{
			"time after boundary value defined as float",
			[]interface{}{float64(time.Now().Add(-1 * time.Hour).Unix())},
			nil,
			true,
		},
		{
			"time before the boundary in location",
			[]interface{}{"2021-02-18T01:00:00", "Europe/Berlin"},
			func() time.Time {
				t, _ := time.Parse(time.RFC3339, "2021-02-17T23:59:00Z")
				return t
			},
			false,
		},
		{
			"time after the boundary in location",
			[]interface{}{"2021-02-18T01:00:00", "Europe/Berlin"},
			func() time.Time {
				t, _ := time.Parse(time.RFC3339, "2021-02-18T00:01:00Z")
				return t
			},
			true,
		},
	}

	for _, c := range cases {
		p, err := NewAfter().Create(c.args)
		if err != nil {
			t.Errorf("Failed to create predicate: %q", err)
		} else {
			afterPredicate := p.(*predicate)
			if c.getTime != nil {
				afterPredicate.getTime = c.getTime
			}

			matches := afterPredicate.Match(request)

			if matches != c.matches {
				t.Errorf("%q: Expected result - %t; Actual result - %t", c.msg, c.matches, matches)
			}
		}
	}
}
