/*
Package loadbalancer implements a predicate which will match for different backends
in a round-robin fashion.

First parameter defines a group which determines the set of possible routes to match.

Second parameter is 0-based index of the route among the other routes in the same group.

Third parameter is the total number of routes in the group.

Eskip example:

	LoadBalancer("group-name", 0, 2) -> "https://www.example.org:8000";
	LoadBalancer("group-name", 1, 2) -> "https://www.example.org:8001";

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
