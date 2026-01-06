package auth

import (
	"testing"
)

const (
	testToken                    = "test-token"
	testWebhookInvalidScopeToken = "test-webhook-invalid-scope-token"
	testUID                      = "jdoe"
	testScope                    = "test-scope"
	testScope2                   = "test-scope2"
	testScope3                   = "test-scope3"
	testRealmKey                 = "/realm"
	testRealm                    = "/immortals"
	testKey                      = "uid"
	testValue                    = "jdoe"
	testAuthPath                 = "/test-auth"
	testSub                      = "somesub"
)

func Test_all(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		l        []string
		r        []string
		expected bool
	}{{
		msg:      "l and r nil",
		l:        nil,
		r:        nil,
		expected: true,
	}, {
		msg:      "l is nil and r has one",
		l:        nil,
		r:        []string{"s1"},
		expected: true,
	}, {
		msg:      "l has one and r has one, same",
		l:        []string{"s1"},
		r:        []string{"s1"},
		expected: true,
	}, {
		msg:      "l has one and r has one, different",
		l:        []string{"l"},
		r:        []string{"r"},
		expected: false,
	}, {
		msg:      "l has one and r has two, one same different 1",
		l:        []string{"l"},
		r:        []string{"l", "r"},
		expected: true,
	}, {
		msg:      "l has one and r has two, one same different 2",
		l:        []string{"l"},
		r:        []string{"r", "l"},
		expected: true,
	}, {
		msg:      "l has two and r has two, one different",
		l:        []string{"l", "l2"},
		r:        []string{"r", "l"},
		expected: false,
	}, {
		msg:      "l has two and r has two, both same 1",
		l:        []string{"l", "r"},
		r:        []string{"r", "l"},
		expected: true,
	}, {
		msg:      "l has two and r has two, both same 2",
		l:        []string{"r", "l"},
		r:        []string{"r", "l"},
		expected: true,
	}, {
		msg:      "l has N and r has M, r has all of left",
		l:        []string{"r1", "l"},
		r:        []string{"r2", "l", "r1"},
		expected: true,
	}, {
		msg:      "l has N and r has M, l has all of right",
		l:        []string{"l1", "r1", "l2"},
		r:        []string{"r1", "l1"},
		expected: false,
	}, {
		msg:      "l has N and r has M, l is missing one of r",
		l:        []string{"r1", "l1"},
		r:        []string{"r1", "l1", "r2"},
		expected: true,
	}, {
		msg:      "l has N and r has M, r is missing one of l",
		l:        []string{"r1", "l1", "l2"},
		r:        []string{"r1", "l1", "r2"},
		expected: false,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			if all(ti.l, ti.r) != ti.expected {
				t.Errorf("Failed test: %s", ti.msg)
			}
		})

	}
}
