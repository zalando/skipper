/*
Package tee provides a unix-like tee feature for routing.

Using this filter, the request will be sent to a "shadow" backend in addition to the main
backend of the route.

Example:

	* -> tee("https://audit-logging.example.org") -> "https://foo.example.org"

This will send an identical request for foo.example.org to audit-logging.example.org.
Another use case could be using it for benchmarking a new backend with some real traffic

The above route will forward the request to https://foo.example.org as it normally would do,
but in addition to that, it will send an identical request to https://audit-logging.example.org.
The request sent to https://audit-logging.example.org will receive the same method and headers,
and a copy of the body stream. The tee response is ignored.

It is possible to change the path of the tee request, in a similar way to the modPath filter:

	Path("/api/v1") -> tee("https://api.example.org", "^/v1", "/v2" ) -> "http://api.example.org"

In the above example, one can test how a new version of an API would behave on incoming requests.
*/
package tee
