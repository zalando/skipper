package rfc

import "testing"

func TestPatch(t *testing.T) {
	type test struct{ title, parsed, raw, expected string }
	for _, test := range []test{{
		title: "empty",
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
