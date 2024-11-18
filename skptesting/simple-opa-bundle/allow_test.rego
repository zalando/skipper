package envoy.authz_test

import rego.v1

import data.envoy.authz

test_not_allowed if {
	not authz.allow with input as {
		"parsed_path": [
			"some-path"
		],
	}
}

test_allowed if {
	authz.allow with input as {
		"parsed_path": [
			"allow"
		],
	}
}
