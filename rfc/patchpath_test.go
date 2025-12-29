package rfc

import "testing"

func TestPatch(t *testing.T) {
	type test struct{ title, parsed, raw, expected string }
	for _, test := range []test{{
		title: "empty",
	}, {
		title:    "not escaped, empty raw",
		parsed:   "/foo/bar",
		expected: "/foo/bar",
	}, {
		title:    "already escaped, empty raw (invalid case)",
		parsed:   "/foo%2Fbar",
		expected: "/foo%2Fbar",
	}, {
		title: "only raw (invalid case)",
		raw:   "/foo/bar",
	}, {
		title:    "not reserved",
		raw:      "/foo%2Abar",
		parsed:   "/foo*bar",
		expected: "/foo*bar",
	}, {
		title:    "reserved",
		raw:      "/foo%2Fbar",
		parsed:   "/foo/bar",
		expected: "/foo%2Fbar",
	}, {
		title:    "reserved, lowercase",
		raw:      "/foo%2fbar",
		parsed:   "/foo/bar",
		expected: "/foo%2fbar",
	}, {
		title:    "modified, too short",
		raw:      "/foo%2Fbar",
		parsed:   "/foo/",
		expected: "/foo/",
	}, {
		title:    "modified, too short, before escape",
		raw:      "/foo%2Fbar",
		parsed:   "/foo",
		expected: "/foo",
	}, {
		title:    "modified, too long",
		raw:      "/foo%2Fbar",
		parsed:   "/foo/bar/baz",
		expected: "/foo/bar/baz",
	}, {
		title:    "modified, different",
		raw:      "/foo%2Fbar",
		parsed:   "/foo/baz",
		expected: "/foo/baz",
	}, {
		title:    "modified, different escaped",
		raw:      "/foo%2Fbar",
		parsed:   "/foo*bar",
		expected: "/foo*bar",
	}, {
		title:    "damaged raw (invalid case)",
		raw:      "/foo%2",
		parsed:   "/foo/",
		expected: "/foo/",
	}, {
		title:    "reserved and unreserved",
		raw:      "/foo%2Fbar/%2A",
		parsed:   "/foo/bar/*",
		expected: "/foo%2Fbar/*",
	}, {
		title:    "unreserved and reserved",
		raw:      "/foo%2Abar%2F",
		parsed:   "/foo*bar/",
		expected: "/foo*bar%2F",
	}, {
		title:    "non-ascii range",
		raw:      "/世%2F界",
		parsed:   "/世/界",
		expected: "/世%2F界",
	}} {
		t.Run(test.title, func(t *testing.T) {
			patched := PatchPath(test.parsed, test.raw)
			if patched != test.expected {
				t.Errorf(
					"patched: %s, %s; got: %s, expected: %s",
					test.parsed, test.raw, patched, test.expected,
				)
			}
		})
	}
}
