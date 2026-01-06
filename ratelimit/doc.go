/*
Package ratelimit implements rate limiting functionality for the proxy.

It provides per process rate limiting. It can be
configured globally, or based on routes. Rate limiting can be looked up
based on HTTP headers for example X-Forwarded-For or Authorization.

# Lookuper Type - SameBucketLookuper

This lookuper will use a static string to point always to the same
bucket. This means all requests are counted the same.

# Lookuper Type - HeaderLookuper

This lookuper will use the content of the specified header to
calculate rate limiting.

# Lookuper Type - XForwardedForLookuper

This lookuper will use the remote IP of the origin request to
calculate rate limiting. If there is no such header it will use the
remote IP of the request. This is the default Lookuper and may be the
one most users want to use.

# Usage

When imported as a package, the Registry can be used to hold the rate
limiters and their settings. On a higher level, rate limiter settings
can be simply passed to skipper as part of the skipper.Options object,
or defined as command line flags.

The following command starts skipper with default X-Forwarded-For
Lookuper, that will start to rate limit after 5 requests within 60s
from the same client

	% skipper -ratelimits type=client,max-hits=5,time-window=60s

The following configuration will rate limit /foo after 2 requests
within 90s from the same requester and all other requests after 20
requests within 60s from the same client

	% cat ratelimit.eskip
	foo: Path("/foo") -> clientRatelimit(2,"1m30s") -> "http://www.example.org/foo"
	rest: * -> clientRatelimit(20,"1m") -> "http://www.example.net/"
	% skipper -enable-ratelimits -routes-file=ratelimit.eskip

The following configuration will rate limit requests after 100
requests within 1 minute with the same Authorization Header

	% cat ratelimit-auth.eskip
	all: * -> clientRatelimit(100,"1m","Authorization") -> "http://www.example.org/"
	% skipper -enable-ratelimits -routes-file=ratelimit-auth.eskip

The following configuration will rate limit requests to /login after 10 requests
summed across all skipper peers within one hour from the same requester.

	% cat ratelimit.eskip
	foo: Path("/login") -> clientRatelimit(10,"1h") -> "http://www.example.org/login"
	rest: * -> "http://www.example.net/"
	% skipper -enable-ratelimits -routes-file=ratelimit.eskip -enable-swarm

Rate limiter settings can be applied globally via command line flags
or within routing settings.

# Settings - Type

Defines the type of the rate limiter. There are types that only use
local state information and others that use cluster information using
swarm.Swarm or redis ring shards to exchange information. Types that
use instance local information are ServiceRatelimit to be used to
protect backends and ClientRatelimit to protect from too chatty
clients. Types that use cluster information are
ClusterServiceRatelimit to be used to protect backends and and
ClusterClientRatelimit to protect from too chatty
clients. ClusterClientRatelimit using swarm should be carefully tested
with your current memory settings (about 15MB for 100.000 attackers
per filter), but the use cases are to protect from login attacks, user
enumeration or DDoS attacks. ClusterServiceRatelimit and
ClusterClientRatelimit using redis ring shards should be carefully
tested, because of redis. Redis ring based cluster ratelimits should
not create a significant memory footprint for skipper instances, but
might create load to redis.

# Settings - MaxHits

Defines the maximum number of requests per user within a TimeWindow.

# Settings - TimeWindow

Defines the time window until rate limits will be enforced, if maximum
number of requests are exceeded. This is defined as a string
representation of Go's time.Duration, e.g. 1m30s.

# Settings - Lookuper

Defines an optional configuration to choose which Header should be
used to group client requests. It accepts any header, for example
"Authorization".

# Settings - Group

Defines the ratelimit group, which can be the same for different
routes, if you want to have one ratelimiter spanning more than one
route. Make sure your settings are the same for the whole group. In
case of different settings for the same group the behavior is
undefined and could toggle between different configurations.

# HTTP Response

In case of rate limiting, the HTTP response status will be 429 Too
Many Requests and two headers will be set.

One which shows the maximum requests per hour:

	X-Rate-Limit: 6000

And another indicating how long (in seconds) to wait before making a new
request:

	Retry-After: 3600

Both are based on RFC 6585.

# Registry

The active rate limiters are stored in a registry. They are created
based on routes or command line flags. The registry synchronizes
access to the shared rate limiters. A registry has default settings
that it will apply and that it will use the disable rate limiter in
case it's not defined in the configuration or not global enabled.
*/
package ratelimit
