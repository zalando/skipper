/*
Package circuit implements circuit breaker functionality for the proxy.

It provides two types of circuit breakers: consecutive and failure rate based. The circuit breakers can be
configured either globally, based on hosts or individual routes. The registry ensures synchronized access to the
active breakers and the recycling of the idle ones.

The circuit breakers are always assigned to backend hosts, so that the outcome of requests to one host never
affects the circuit breaker behavior of another host. Besides hosts, individual routes can have separate circuit
breakers, too.

# Breaker Type - Consecutive Failures

This breaker opens when the proxy couldn't connect to a backend or received a >=500 status code at least N times
in a row. When open, the proxy returns 503 - Service Unavailable response during the breaker timeout. After this
timeout, the breaker goes into half-open state, in which it expects that M number of requests succeed. The
requests in the half-open state are accepted concurrently. If any of the requests during the half-open state
fails, the breaker goes back to open state. If all succeed, it goes to closed state again.

# Breaker Type - Failure Rate

The "rate breaker" works similar to the "consecutive breaker", but instead of considering N consecutive failures
for going open, it maintains a sliding window of the last M events, both successes and failures, and opens only
when the number of failures reaches N within the window. This way the sliding window is not time based and
allows the same breaker characteristics for high and low rate traffic.

# Usage

When imported as a package, the Registry can be used to hold the circuit breakers and their settings. On a
higher level, the circuit breaker settings can be simply passed to skipper as part of the skipper.Options
object, or, equivalently, defined as command line flags.

The following command starts skipper with a global consecutive breaker that opens after 5 failures for any
backend host:

	skipper -breaker type=consecutive,failures=5

To set only the type of the breaker globally, and the rates individually for the hosts:

	skipper -breaker type=rate,timeout=3m,idleTTL=30m \
		-breaker host=foo.example.org,window=300,failures=30 \
		-breaker host=bar.example.org,window=120,failures=45

To enable circuit breakers only for specific hosts:

	skipper -breaker type=disabled \
		-breaker type=rate,host=foo.example.org,window=300,failures=30

To change (or set) the breaker configurations for an individual route and disable for another, in eskip:

	updates: Method("POST") && Host("foo.example.org")
	  -> consecutiveBreaker(9)
	  -> "https://foo.backend.net";

	backendHealthcheck: Path("/healthcheck")
	  -> disableBreaker()
	  -> "https://foo.backend.net";

The breaker settings can be defined in the following levels: global, based on the backend host, based on
individual route settings. The values are merged in the same order, so the global settings serve as defaults for
the host settings, and the result of the global and host settings serve as defaults for the route settings.
Setting global values happens the same way as setting host values, but leaving the Host field empty.  Setting
route based values happens with filters in the route definitions.
(https://pkg.go.dev/github.com/zalando/skipper/filters/circuit)

# Settings - Type

It can be ConsecutiveFailures, FailureRate or Disabled, where the first two values select which breaker to use,
while the Disabled value can override a global or host configuration disabling the circuit breaker for the
specific host or route.

Command line name: type. Possible command line values: consecutive, rate, disabled.

# Settings - Host

The Host field indicates to which backend host should the current set of settings be applied. Leaving it empty
indicates global settings.

Command line name: host.

# Settings - Window

The window value sets the size of the sliding counter window of the failure rate breaker.

Command line name: window. Possible command line values: any positive integer.

# Settings - Failures

The failures value sets the max failure count for both the "consecutive" and "rate" breakers.

Command line name: failures. Possible command line values: any positive integer.

# Settings - Timeout

With the timeout we can set how long the breaker should stay open, before becoming half-open.

Command line name: timeout. Possible command line values: any positive integer as milliseconds or a duration
string, e.g. 15m30s.

# Settings - Half-Open Requests

Defines the number of requests expected to succeed while the circuit breaker is in the half-open state.

Command line name: half-open-requests. Possible command line values: any positive integer.

# Settings - Idle TTL

Defines the idle timeout after which a circuit breaker gets recycled, if it hasn't been used.

Command line name: idle-ttl. Possible command line values: any positive integer as milliseconds or a duration
string, e.g. 15m30s.

# Filters

The following circuit breaker filters are supported: consecutiveBreaker(), rateBreaker() and disableBreaker().

The consecutiveBreaker filter expects one mandatory parameter: the number of consecutive failures to open. It
accepts the following optional arguments: timeout, half-open requests, idle-ttl.

	consecutiveBreaker(5, "1m", 12, "30m")

The rateBreaker filter expects two mandatory parameters: the number of consecutive failures to open and the size
of the sliding window. It accepts the following optional arguments: timeout, half-open requests, idle-ttl.

	rateBreaker(30, 300, "1m", 12, "30m")

The disableBreaker filter doesn't expect any arguments, and it disables the circuit breaker, if any, for the
route that it appears in.

	disableBreaker()

# Proxy Usage

The proxy, when circuit breakers are configured, uses them for backend connections. It checks the breaker for
the current backend host if it's closed before making backend requests. It reports the outcome of the request to
the breaker, considering connection failures and backend responses with status code >=500 as failures. When the
breaker is open, the proxy doesn't try to make backend requests, and returns a response with a status code of
503 and appending a header to the response:

	X-Circuit-Open: true

# Registry

The active circuit breakers are stored in a registry. They are created on-demand, for the requested settings.
The registry synchronizes access to the shared circuit breakers. When the registry detects that a circuit
breaker is idle, it resets it, this way avoiding that an old series of failures would cause the circuit breaker
go open after an unreasonably low number of recent failures. The registry also makes sure to cleanup idle
circuit breakers that are not requested anymore by the proxy. This happens in a passive way, whenever a new
circuit breaker is created. The cleanup prevents storing circuit breakers for inaccessible backend hosts
infinitely in those scenarios where the route configuration is continuously changing.
*/
package circuit
