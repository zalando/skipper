/*
Package loadbalancer implements load balancer algorithms that are applied by the proxy.

roundRobin Algorithm

	The roundRobin algorithm does proxy requests round robin to
	backend endpoints. It has a mutex to update the index and will
	start at a random index

random Algorithm

	The random algorithm does proxy requests to random backend
	endpoints.

consistentHash Algorithm

	The consistentHash algorithm choose backend endpoints by hashing
	client data with hash function fnv.New32. The client data is the
	client IP, which will be looked up from X-Forwarded-For header
	with remote IP as the fallback.

powerOfRandomNChoices Algorithm

	The powerOfRandomNChoices algorithm selects N random endpoints
	and picks the one with least outstanding requests from them.
	Currently, N is 2.

The load balancing algorithms also provide fade-in behavior for LB endpoints of routes where the
fade-in duration was configured. This feature can be used to gradually add traffic to new instances of
applications that require a certain amount of warm-up time.

Eskip example:

	r1: * -> <roundRobin, "http://127.0.0.1:9998", "http://127.0.0.1:9997">;
	r2: * -> <consistentHash, "http://127.0.0.1:9998", "http://127.0.0.1:9997">;
	r3: * -> <random, "http://127.0.0.1:9998", "http://127.0.0.1:9997">;
	r4: * -> <powerOfRandomNChoices, "http://127.0.0.1:9998", "http://127.0.0.1:9997">;

Package loadbalancer also implements health checking of pool members for
a group of routes, if backend calls are reported to the loadbalancer.

Based on https://landing.google.com/sre/book/chapters/load-balancing-datacenter.html#identifying-bad-tasks-flow-control-and-lame-ducks-bEs0uy we use

Healthy (healthy)

	The backend task has initialized correctly and is processing
	requests.

Refusing connections (dead)

	The backend task is unresponsive. This can happen because the
	task is starting up or shutting down, or because the backend is
	in an abnormal state (though it would be rare for a backend to
	stop listening on its port if it is not shutting down).

Lame duck (unhealthy)

	The backend task is listening on its port and can serve, but is
	explicitly asking clients to stop sending requests.
*/
package loadbalancer
