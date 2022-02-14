package host

import (
	"fmt"
	"net/http"
	"testing"
)

func TestHostAnyArgs(t *testing.T) {
	s := NewAny()
	for _, tc := range []struct {
		args []interface{}
	}{
		{
			args: []interface{}{},
		},
		{
			args: []interface{}{1.2},
		},
		{
			args: []interface{}{"example.org", 3.4},
		},
	} {
		if _, err := s.Create(tc.args); err == nil {
			t.Errorf("expected error for arguments: %v", tc.args)
		}
	}
}

func TestHostAnyMatch(t *testing.T) {
	s := NewAny()
	for _, tc := range []struct {
		host  string
		args  []interface{}
		match bool
	}{
		{
			host:  "example.org",
			args:  []interface{}{"example.com"},
			match: false,
		},
		{
			host:  "example.org",
			args:  []interface{}{"example.com", "example.net"},
			match: false,
		},
		{
			host:  "example.org",
			args:  []interface{}{"www.example.org"},
			match: false,
		},
		{
			host:  "www.example.org",
			args:  []interface{}{"example.org"},
			match: false,
		},
		{
			host:  "example.org.",
			args:  []interface{}{"example.org"},
			match: false,
		},
		{
			host:  "example.org:8080",
			args:  []interface{}{"example.org"},
			match: false,
		},
		{
			host:  "example.org.:8080",
			args:  []interface{}{"example.org"},
			match: false,
		},
		{
			host:  "example.org",
			args:  []interface{}{"example.org"},
			match: true,
		},
		{
			host:  "example.org:8080",
			args:  []interface{}{"example.org:8080"},
			match: true,
		},
		{
			host:  "example.org",
			args:  []interface{}{"example.org", "example.com"},
			match: true,
		},
		{
			host:  "example.org",
			args:  []interface{}{"example.com", "example.org"},
			match: true,
		},
		{
			host:  "example.org:8080",
			args:  []interface{}{"example.org", "example.org:8080"},
			match: true,
		},
	} {
		t.Run(fmt.Sprintf("%s->%v", tc.host, tc.args), func(t *testing.T) {
			p, err := s.Create(tc.args)
			if err != nil {
				t.Error(err)
			}
			if p.Match(&http.Request{Host: tc.host}) != tc.match {
				t.Errorf("expected match: %v", tc.match)
			}
		})
	}
}
