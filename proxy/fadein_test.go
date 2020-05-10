package proxy_test

import "testing"

/*
The following tests address the fade-in behavior expectations as observed from the perspective of the backend
endpoints of a single route.

Available data:
- fade-in configuration: this contains the fade-in duration and optionally the fade-in degree.
- detection time: this is the time when Skipper detected an endpoint that it didn't know about.
- creation time, optional: this is external information about the creation of an endpoint, when available. E.g.
  it can be the time when a Kubernetes pod became ready. It can be used as a fallback value for the detection
  time in cases when Skipper (re)starts. When present, it renders any detection time past this value as invalid.
- synced detection time, optional: it's the detection time of an endpoint shared across multiple Skipper
  instances, stored in Redis or shared as a Skipper swarm datagram. It can be used to continue the fade-in when
  Skipper gets (re)started. It has a precedence over the external creation time in these cases.

States, all the combinations of:
- single or multiple proxy instances
- proxies with or without syncing
- zero, one or more endpoints
- endpoints with or without fade-in
- endpoints with or without available creation time

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
	no endpoints, with/without fade-in, with/without creation time
		start single/multiple -> no fade-in
	single endpoint
		with fade-in
			with/without creation time
				start single/multiple -> fade-in
				restart -> no fade-in
				restart proxy -> no fade-in
			with creation time
				restart detected through creation time -> no fade-in
				restart proxy during fade-in single/multiple -> continue fade-in
			without creation time
				restart proxy during fade-in single/multiple -> no fade-in
		without fade-in
			with/without creation time
				start single/multiple -> no fade-in
				restart -> no fade-in
				restart proxy -> no fade-in
			with creation time
				restart detected through creation time -> no fade-in
	multiple endpoints
		with fade-in
			with/without creation time
				start single/multiple -> fade-in
				restart some -> fade-in
				restart all -> no fade-in
				delete some endpoints -> no fade-in
				delete some endpoints while fade-in -> adjust fade-in
				restart proxy -> no fade-in
			with creation time
				restart some detected through creation time -> fade-in
				restart all detected through creation time -> no fade-in
				restart proxy during fade-in single/multiple -> continue fade-in
			without creation time
				restart proxy during fade-in single/multiple -> no fade-in
		without fade-in
			with/without creation time
				start single/multiple -> no fade-in
				restart some/all -> no fade-in
				delete some endpoints -> no fade-in
				restart proxy -> no fade-in
			with creation time
				restart some/all detected through creation time -> no fade-in

single proxy, with sync
	no endpoitns, with/without fade-in, with/without creation time
		start single/mutliple -> no fade-in
	single endpoint
		with fade-in
			with/without creation time
				start single/multiple -> fade-in
				restart -> no fade-in
				restart proxy -> no fade-in
				restart proxy during fade-in single/multiple -> continue fade-in
			with creation time
				restart detected through creation time -> no fade-in
		without fade-in
			with/without creation time
				start single/multiple -> no fade-in
				restart -> no fade-in
				restart proxy -> no fade-in
				restart proxy during fade-in single/multiple -> no fade-in
			with creation time
				restart detected through creation time -> no fade-in
	multiple endpoints
		with fade-in
			with/without creation time
				start single/multiple -> fade-in
				restart some -> fade-in
				restart all -> no fade-in
				delete some endpoints -> no fade-in
				delete some endpoints while fade-in -> adjust fade-in
				restart proxy -> no fade-in
				restart proxy during fade-in single/multiple -> continue fade-in
			with creation time
				restart some detected through creation time -> fade-in
				restart all detected through creation time -> no fade-in
		without fade-in
			with/without creation time
				start single/multiple -> no fade-in
				restart some/all -> no fade-in
				delete some endpoints -> no fade-in
				restart proxy -> no fade-in
			with creation time
				restart some/all detected through creation time -> no fade-in

multiple proxies, without sync
	no endpoints, with/without fade-in, with/without creation time
		start single/multiple -> no fade-in
	single endpoint
		with fade-in
			with/without creation time
				start single/multiple -> fade-in
				restart -> no fade-in
				restart some/all proxies -> no fade-in
				restart some/all proxies during fade-in single/multiple -> continue fade-in
			with creation time
				restart detected through creation time -> no fade-in
		without fade-in
			with/without creation time
				start single/multiple -> no fade-in
				restart -> no fade-in
				restart some/all proxies -> no fade-in
			with creation time
				restart detected through creation time -> no fade-in
	multiple endpoints
		with fade-in
			with/without creation time
				start single/multiple -> fade-in
				restart some -> fade-in
				restart all -> no fade-in
				delete some endpoints -> no fade-in
				delete some endpoints while fade-in -> adjust fade-in
				restart some/all proxies -> no fade-in
			with creation time
				restart some detected through creation time -> fade-in
				restart all detected through creation time -> no fade-in
				restart some/all proxies during fade-in single/multiple -> continue fade-in
			without creation time
				restart some/all proxies during fade-in single/multiple -> no fade-in
		without fade-in
			with/without creation time
				start single/multiple -> no fade-in
				restart some/all -> no fade-in
				delete some endpoints -> no fade-in
				restart some/all proxies -> no fade-in
			with creation time
				restart some/all detected through creation time -> no fade-in

multiple proxies, with sync
	no endpoints, with/without fade-in, with/without creation time
		start single/multiple -> no fade-in
	single endpoint
		with fade-in
			with/without creation time
				start single/multiple -> fade-in
				restart -> no fade-in
				restart some/all proxies -> no fade-in
				restart some/all proxies during fade-in single/multiple -> continue fade-in
			with creation time
				restart detected through creation time -> no fade-in
		without fade-in
			with/without creation time
				start single/multiple -> no fade-in
				restart -> no fade-in
				restart some/all proxies -> no fade-in
				restart some/all proxies during fade-in single/multiple -> no fade-in
			with creation time
				restart detected through creation time -> no fade-in
	multiple endpoints
		with fade-in
			with/without creation time
				start single/multiple -> fade-in
				restart some -> fade-in
				restart all -> no fade-in
				delete some endpoints -> no fade-in
				delete some endpoints while fade-in -> adjust fade-in
				restart some/all proxies -> no fade-in
				restart some/all proxies during fade-in single/multiple -> continue fade-in
			with creation time
				restart some detected through creation time -> fade-in
				restart all detected through creation time -> no fade-in
		without fade-in
			with/without creation time
				start single/multiple -> no fade-in
				restart some/all -> no fade-in
				delete some endpoints -> no fade-in
				restart some/all proxies -> no fade-in
			with creation time
				restart some/all detected through creation time -> no fade-in
*/

func TestFadeIn(t *testing.T) {
	t.Run("single proxy, no sync", func(t *testing.T) {
		t.Run("no endpoints", func(t *testing.T) {
			t.Run("start single", func(t *testing.T) {
				b := startBackend(t)
				defer b.close()

				p := startProxy(t, b)
				defer p.close()
				p.addInstances(1)

				c := startClient(t, p)
				defer c.close()

				u := randomURLs(t, 1)
				b.addInstances(u...)

				c.warmUp()
				c.checkNoFadeIn(u)
			})

			t.Run("start multiple", func(t *testing.T) {
			})
		})
	})
}
