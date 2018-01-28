/*
Package loadbalancer implements a predicate which will match for different backends
in a round-robin fashion.

First parameter defines a group which determines the set of possible routes to match.

Second parameter is 0-based index of the route among the other routes in the same group.

Third parameter is the total number of routes in the group.

Eskip example:

	LoadBalancer("group-name", 0, 2) -> "https://www.example.org:8000";
	LoadBalancer("group-name", 1, 2) -> "https://www.example.org:8001";
*/
package loadbalancer
