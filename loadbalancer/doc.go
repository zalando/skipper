/*
Package loadbalancer implements predicates and filter which will match for different backends
in a round-robin fashion.

First parameter to LBGroup, lbDecide and LBMember defines a group which determines the set of possible routes to match.

lbDecide's second parameter is the number of members in a loadbalancer group.

LBMember's second parameter is 0-based index of the route among the other routes in the same group.

Eskip example:

	hello_lb_group: Path("/foo") && LBGroup("hello")
	        -> lbDecide("hello", 3)
	        -> <loopback>;
	hello_1: Path("/foo") && LBMember("hello",0)
	        -> "http://127.0.0.1:12345";
	hello_2: Path("/foo") && LBMember("hello",1)
	        -> "http://127.0.0.1:12346";
	hello_3: Path("/foo") && LBMember("hello",2)
	        -> "http://127.0.0.1:12347";


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
