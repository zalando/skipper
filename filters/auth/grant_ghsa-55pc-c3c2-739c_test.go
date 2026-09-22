package auth

import "testing"

// TestAllowedForHostSuffixCollision documents GHSA-55pc-c3c2-739c:
// strings.HasSuffix crosses DNS label boundaries, allowing a cookie issued
// for "example.com" to authenticate requests to "notexample.com".
// ref: https://github.com/zalando/skipper/security/advisories/GHSA-55pc-c3c2-739c
//
// Cases marked VULN demonstrate the pre-fix behavior: they return true but
// should return false. These cases FAIL before the fix and PASS after.
func TestAllowedForHostSuffixCollision(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cookieDomain string
		requestHost  string
		wantAllowed  bool
	}{
		// RemoveSubdomains=0: cookie domain = full creation host, e.g. "foo.skipper.test"
		{name: "rs0/exact match allowed", cookieDomain: "foo.skipper.test", requestHost: "foo.skipper.test", wantAllowed: true},
		{name: "rs0/subdomain of cookie domain allowed", cookieDomain: "foo.skipper.test", requestHost: "bar.foo.skipper.test", wantAllowed: true},
		{name: "rs0/sibling domain rejected", cookieDomain: "foo.skipper.test", requestHost: "bar.skipper.test", wantAllowed: false},
		{name: "rs0/unrelated domain rejected", cookieDomain: "foo.skipper.test", requestHost: "other.test", wantAllowed: false},
		// VULN: HasSuffix("afoo.skipper.test", "foo.skipper.test") == true
		{name: "rs0/suffix collision rejected", cookieDomain: "foo.skipper.test", requestHost: "afoo.skipper.test", wantAllowed: false},

		// RemoveSubdomains=1: cookie domain = parent, e.g. "skipper.test" from "foo.skipper.test"
		{name: "rs1/exact match allowed", cookieDomain: "skipper.test", requestHost: "skipper.test", wantAllowed: true},
		{name: "rs1/direct subdomain allowed", cookieDomain: "skipper.test", requestHost: "foo.skipper.test", wantAllowed: true},
		{name: "rs1/deep subdomain allowed", cookieDomain: "skipper.test", requestHost: "baz.foo.skipper.test", wantAllowed: true},
		{name: "rs1/unrelated domain rejected", cookieDomain: "skipper.test", requestHost: "other.test", wantAllowed: false},
		{name: "rs1/parent TLD rejected", cookieDomain: "skipper.test", requestHost: "test", wantAllowed: false},
		// VULN: HasSuffix("askipper.test", "skipper.test") == true
		{name: "rs1/suffix collision rejected", cookieDomain: "skipper.test", requestHost: "askipper.test", wantAllowed: false},
		// VULN: HasSuffix("foo.askipper.test", "skipper.test") == true
		{name: "rs1/suffix collision via subdomain rejected", cookieDomain: "skipper.test", requestHost: "foo.askipper.test", wantAllowed: false},
		// VULN: the canonical advisory example — HasSuffix("notexample.com", "example.com") == true
		{name: "rs1/example.com suffix collision", cookieDomain: "example.com", requestHost: "notexample.com", wantAllowed: false},

		// RemoveSubdomains=2, fallback: 3-label host, 3-2=1 < 2 → domain = full host "foo.skipper.test"
		{name: "rs2/fallback exact match allowed", cookieDomain: "foo.skipper.test", requestHost: "foo.skipper.test", wantAllowed: true},
		// VULN: same collision as rs0
		{name: "rs2/fallback suffix collision rejected", cookieDomain: "foo.skipper.test", requestHost: "afoo.skipper.test", wantAllowed: false},

		// RemoveSubdomains=2, 4-label host: "bar.foo.skipper.test" → domain = "skipper.test"
		{name: "rs2/deep subdomain of grandparent allowed", cookieDomain: "skipper.test", requestHost: "bar.foo.skipper.test", wantAllowed: true},
		// VULN: HasSuffix("bar.foo.askipper.test", "skipper.test") == true
		{name: "rs2/grandparent suffix collision rejected", cookieDomain: "skipper.test", requestHost: "bar.foo.askipper.test", wantAllowed: false},

		// Edge cases from remediation requirements
		{name: "empty domain never matches", cookieDomain: "", requestHost: "foo.skipper.test", wantAllowed: false},
		{name: "port is stripped from request host", cookieDomain: "skipper.test", requestHost: "foo.skipper.test:8080", wantAllowed: true},
		{name: "IP exact match allowed", cookieDomain: "192.0.2.1", requestHost: "192.0.2.1", wantAllowed: true},
		{name: "IP request host rejects domain suffix rule", cookieDomain: "example.com", requestHost: "1.2.3.4", wantAllowed: false},
		{name: "IPv4-fragment suffix collision rejected", cookieDomain: "2.3.4", requestHost: "1.2.3.4", wantAllowed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &cookie{Domain: tc.cookieDomain}
			got := c.allowedForHost(tc.requestHost)
			if got != tc.wantAllowed {
				t.Fatalf("allowedForHost(%q) with domain %q = %v, want %v",
					tc.requestHost, tc.cookieDomain, got, tc.wantAllowed)
			}
		})
	}
}
