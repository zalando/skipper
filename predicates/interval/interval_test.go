// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
			[]interface{}{1, "2016-01-01T12:00:00+07:00"},
			true,
		},
		{
			"second argument is not a string",
			[]interface{}{"2016-01-01T12:00:00+07:00", 1},
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
	}

	for _, c := range cases {
		_, err := NewBetweenPredicate().Create(c.args)

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
	}

	for _, c := range cases {
		_, err := NewBeforePredicate().Create(c.args)

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
	}

	for _, c := range cases {
		_, err := NewAfterPredicate().Create(c.args)

		if err == nil && c.err || err != nil && !c.err {
			t.Errorf("%q: Is error case - %t; Error - %v", c.msg, c.err, err)
		}
	}
}

func TestMatchBetween(t *testing.T) {
	reqeust := &http.Request{}

	cases := []struct {
		msg     string
		args    []interface{}
		time    func() time.Time
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
			"time equals to begin value",
			[]interface{}{"2016-01-01T12:00:00Z", "2016-02-01T12:00:00Z"},
			func() time.Time {
				return time.Date(2016, 1, 1, 12, 0, 0, 0, time.UTC)
			},
			true,
		},
		{
			"time equals to end value",
			[]interface{}{"2016-01-01T12:00:00Z", "2016-02-01T12:00:00Z"},
			func() time.Time {
				return time.Date(2016, 2, 1, 12, 0, 0, 0, time.UTC)
			},
			true,
		},
		{
			"time before begin value",
			[]interface{}{time.Now().Add(time.Hour).Format(time.RFC3339), time.Now().Add(2 * time.Hour).Format(time.RFC3339)},
			nil,
			false,
		},
		{
			"time afer end value",
			[]interface{}{time.Now().Add(-2 * time.Hour).Format(time.RFC3339), time.Now().Add(-1 * time.Hour).Format(time.RFC3339)},
			nil,
			false,
		},
	}

	for _, c := range cases {
		p, err := NewBetweenPredicate().Create(c.args)
		if err != nil {
			t.Errorf("Failed to create predicate: %q", err)
		} else {
			betweenPredicate := p.(*predicate)
			if c.time != nil {
				betweenPredicate.time = c.time
			}

			matches := betweenPredicate.Match(reqeust)

			if matches != c.matches {
				t.Errorf("%q: Expected result - %t; Actual result - %t", c.msg, c.matches, matches)
			}
		}
	}
}

func TestMatchBefore(t *testing.T) {
	reqeust := &http.Request{}

	cases := []struct {
		msg     string
		args    []interface{}
		time    func() time.Time
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
			"time equals to boundary value",
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
	}

	for _, c := range cases {
		p, err := NewBeforePredicate().Create(c.args)
		if err != nil {
			t.Errorf("Failed to create predicate: %q", err)
		} else {
			beforePredicate := p.(*predicate)
			if c.time != nil {
				beforePredicate.time = c.time
			}

			matches := beforePredicate.Match(reqeust)

			if matches != c.matches {
				t.Errorf("%q: Expected result - %t; Actual result - %t", c.msg, c.matches, matches)
			}
		}
	}
}

func TestMatchAfter(t *testing.T) {
	reqeust := &http.Request{}

	cases := []struct {
		msg     string
		args    []interface{}
		time    func() time.Time
		matches bool
	}{
		{
			"time before the boundary value",
			[]interface{}{time.Now().Add(time.Hour).Format(time.RFC3339)},
			nil,
			false,
		},
		{
			"time equals to boundary value",
			[]interface{}{"2016-01-01T12:00:00Z"},
			func() time.Time {
				return time.Date(2016, 1, 1, 12, 0, 0, 0, time.UTC)
			},
			false,
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
	}

	for _, c := range cases {
		p, err := NewAfterPredicate().Create(c.args)
		if err != nil {
			t.Errorf("Failed to create predicate: %q", err)
		} else {
			afterPredicate := p.(*predicate)
			if c.time != nil {
				afterPredicate.time = c.time
			}

			matches := afterPredicate.Match(reqeust)

			if matches != c.matches {
				t.Errorf("%q: Expected result - %t; Actual result - %t", c.msg, c.matches, matches)
			}
		}
	}
}
