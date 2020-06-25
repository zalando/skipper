package proxy_test

import "testing"

/*
The following tests address the fade-in behavior expectations as observed from the perspective of the backend
endpoints of a single route.

Available data:
- fade-in configuration: this contains the fade-in duration and optionally the fade-in degree.
- detection time: this is the time when Skipper detected an endpoint that it didn't know about.
- created time, optional: this is external information about the created of an endpoint, when available. E.g.
  it can be the time when a Kubernetes pod became ready. It can be used as a fallback value for the detection
  time in cases when Skipper (re)starts. When present, it renders any detection time past this value as invalid.
- synced detection time, optional: it's the detection time of an endpoint shared across multiple Skipper
  instances, stored in Redis or shared as a Skipper swarm datagram. It can be used to continue the fade-in when
  Skipper gets (re)started. It has a precedence over the external created time in these cases.

States, all the combinations of:
- single or multiple proxy instances
- proxies with or without syncing
- zero, one or more endpoints
- endpoints with or without fade-in
- endpoints with or without available created time

Events:
- endpoint created
- endpoint detected
- endpoint restarted
- endpoint restarted fast
- endpoint deleted
- proxy (re)started
- proxy (re)started during fade-in

Tests:

single proxy, without sync
	no endpoints, with/without fade-in, with/without created time
		start single/multiple -> no fade-in
	single endpoint
		with fade-in
			with/without created time
				start single/multiple -> fade-in
				restart -> no fade-in
				restart proxy -> no fade-in
			with created time
				restart detected through created time -> no fade-in
				restart proxy during fade-in single/multiple -> continue fade-in
			without created time
				restart proxy during fade-in single/multiple -> no fade-in
		without fade-in
			with/without created time
				start single/multiple -> no fade-in
				restart -> no fade-in
				restart proxy -> no fade-in
			with created time
				restart detected through created time -> no fade-in
	multiple endpoints
		with fade-in
			with/without created time
				start single/multiple -> fade-in
				restart some -> fade-in
				restart all -> no fade-in
				delete some endpoints -> no fade-in
				delete some endpoints while fade-in -> adjust fade-in
				restart proxy -> no fade-in
			with created time
				restart some detected through created time -> fade-in
				restart all detected through created time -> no fade-in
				restart proxy during fade-in single/multiple -> continue fade-in
			without created time
				restart proxy during fade-in single/multiple -> no fade-in
		without fade-in
			with/without created time
				start single/multiple -> no fade-in
				restart some/all -> no fade-in
				delete some endpoints -> no fade-in
				restart proxy -> no fade-in
			with created time
				restart some/all detected through created time -> no fade-in

single proxy, with sync
	no endpoitns, with/without fade-in, with/without created time
		start single/mutliple -> no fade-in
	single endpoint
		with fade-in
			with/without created time
				start single/multiple -> fade-in
				restart -> no fade-in
				restart proxy -> no fade-in
				restart proxy during fade-in single/multiple -> continue fade-in
			with created time
				restart detected through created time -> no fade-in
		without fade-in
			with/without created time
				start single/multiple -> no fade-in
				restart -> no fade-in
				restart proxy -> no fade-in
				restart proxy during fade-in single/multiple -> no fade-in
			with created time
				restart detected through created time -> no fade-in
	multiple endpoints
		with fade-in
			with/without created time
				start single/multiple -> fade-in
				restart some -> fade-in
				restart all -> no fade-in
				delete some endpoints -> no fade-in
				delete some endpoints while fade-in -> adjust fade-in
				restart proxy -> no fade-in
				restart proxy during fade-in single/multiple -> continue fade-in
			with created time
				restart some detected through created time -> fade-in
				restart all detected through created time -> no fade-in
		without fade-in
			with/without created time
				start single/multiple -> no fade-in
				restart some/all -> no fade-in
				delete some endpoints -> no fade-in
				restart proxy -> no fade-in
			with created time
				restart some/all detected through created time -> no fade-in

multiple proxies, without sync
	no endpoints, with/without fade-in, with/without created time
		start single/multiple -> no fade-in
	single endpoint
		with fade-in
			with/without created time
				start single/multiple -> fade-in
				restart -> no fade-in
				restart some/all proxies -> no fade-in
				restart some/all proxies during fade-in single/multiple -> continue fade-in
			with created time
				restart detected through created time -> no fade-in
		without fade-in
			with/without created time
				start single/multiple -> no fade-in
				restart -> no fade-in
				restart some/all proxies -> no fade-in
			with created time
				restart detected through created time -> no fade-in
	multiple endpoints
		with fade-in
			with/without created time
				start single/multiple -> fade-in
				restart some -> fade-in
				restart all -> no fade-in
				delete some endpoints -> no fade-in
				delete some endpoints while fade-in -> adjust fade-in
				restart some/all proxies -> no fade-in
			with created time
				restart some detected through created time -> fade-in
				restart all detected through created time -> no fade-in
				restart some/all proxies during fade-in single/multiple -> continue fade-in
			without created time
				restart some/all proxies during fade-in single/multiple -> no fade-in
		without fade-in
			with/without created time
				start single/multiple -> no fade-in
				restart some/all -> no fade-in
				delete some endpoints -> no fade-in
				restart some/all proxies -> no fade-in
			with created time
				restart some/all detected through created time -> no fade-in

multiple proxies, with sync
	no endpoints, with/without fade-in, with/without created time
		start single/multiple -> no fade-in
	single endpoint
		with fade-in
			with/without created time
				start single/multiple -> fade-in
				restart -> no fade-in
				restart some/all proxies -> no fade-in
				restart some/all proxies during fade-in single/multiple -> continue fade-in
			with created time
				restart detected through created time -> no fade-in
		without fade-in
			with/without created time
				start single/multiple -> no fade-in
				restart -> no fade-in
				restart some/all proxies -> no fade-in
				restart some/all proxies during fade-in single/multiple -> no fade-in
			with created time
				restart detected through created time -> no fade-in
	multiple endpoints
		with fade-in
			with/without created time
				start single/multiple -> fade-in
				restart some -> fade-in
				restart all -> no fade-in
				delete some endpoints -> no fade-in
				delete some endpoints while fade-in -> adjust fade-in
				restart some/all proxies -> no fade-in
				restart some/all proxies during fade-in single/multiple -> continue fade-in
			with created time
				restart some detected through created time -> fade-in
				restart all detected through created time -> no fade-in
		without fade-in
			with/without created time
				start single/multiple -> no fade-in
				restart some/all -> no fade-in
				delete some endpoints -> no fade-in
				restart some/all proxies -> no fade-in
			with created time
				restart some/all detected through created time -> no fade-in
*/

func TestFadeIn(t *testing.T) {
	run(t,
		"single proxy, no sync",
		sub(
			"no endpoints",
			sub(
				"with fade-in",
				sub(
					"with created time",
					sub("start single", endpointStartTest(1, 0, 1, true, true, false)),
					sub("start multiple", endpointStartTest(1, 0, 3, true, true, false)),
				),
				sub(
					"without created time",
					sub("start single", endpointStartTest(1, 0, 1, true, false, false)),
					sub("start multiple", endpointStartTest(1, 0, 3, true, false, false)),
				),
			),
			sub(
				"without fade-in",
				sub(
					"with created time",
					sub("start single", endpointStartTest(1, 0, 1, false, true, false)),
					sub("start multiple", endpointStartTest(1, 0, 3, false, true, false)),
				),
				sub(
					"without created time",
					sub("start single", endpointStartTest(1, 0, 1, false, false, false)),
					sub("start multiple", endpointStartTest(1, 0, 3, false, false, false)),
				),
			),
		),
		sub(
			"single endpoint",
			sub(
				"with fade-in",
				sub(
					"with created time",
					sub("start single", endpointStartTest(1, 1, 1, true, true, true)),
					sub("start multiple", endpointStartTest(1, 1, 3, true, true, true)),
				),
			),
		),
	)
}
