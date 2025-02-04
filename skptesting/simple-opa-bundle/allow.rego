package envoy.authz

import rego.v1

default allow := false

# METADATA
# entrypoint: true
allow if {
	input.parsed_path = [ "allow" ]
}
