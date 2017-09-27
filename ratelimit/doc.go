/*
Package ratelimit implements rate limiting functionality for the proxy.

It provides per process rate limiting. It can be
configured globally, or based on routes. Rate limiting can be lookuped
based on HTTP headers like X-Forwarded-For or Authorization.

Lookuper Type - Authorization Header

This lookuper will use the content of the Authorization header to
calculate rate limiting. This will work for Bearer tokens or Basic
Auth without change of the rate limiter configuration.

Lookuper Type - X-Forwarded-For Header

This lookuper will use the remote IP of the origin request to
calculate rate limiting. If there is no such header it will use the
remote IP of the request. This is the default Lookuper and may be the
one most users want to use.

Usage

When imported as a package, the Registry can be used to hold the rate
limiters and their settings On a higher level, rate limiter settings
can be simply passed to skipper as part of the skipper.Options object,
or defined as command line flags.

The following command starts skipper with default X-Forwarded-For
Lookuper, that will start to rate limit after 5 requests within 60s
from the same requester

    % skipper -ratelimits type=local,max-hits=5,time-window=60s

The following configuration will rate limit /foo after 2 requests
within 90s from the same requester and all other requests after 20
requests within 60s from the same requester

    % cat ratelimit.eskip
    foo: Path("/foo") -> localRatelimit(2,"1m30s") -> "http://www.example.org/foo"
    rest: Path("/") -> localRatelimit(20,"1m") -> "http://www.example.net/"
    % skipper -enable-ratelimits -routes-file=ratelimit.eskip

The following configuration will rate limit requests after 100
requests within 1 minute with the same Authorization Header

    % cat ratelimit-auth.eskip
    all: Path("/") -> localRatelimit(100,"1m","auth") -> "http://www.example.org/"
    % skipper -enable-ratelimits -routes-file=ratelimit-auth.eskip

Rate limiter settings can be applied globally via command line flags
or within routing settings.

Settings - Type

Defines the type of the rate limiter, which right now only allows to
be "local". In case of a skipper swarm or service mesh this would be
an interesting configuration option, for example "global" or "dc".

Settings - MaxHits

Defines the maximum number of requests per user within a TimeWindow.

Settings - TimeWindow

Defines the time window until rate limits will be forced, if maximum
number of requests are exceeded. This is defined as string
representation of Go's time.Duration, e.g. 1m30s.

Settings - Lookuper

Defines an optional configuration to choose which Header should be
used to group client requests. It accepts the default
"x-forwarded-for" or "auth"

HTTP Response

In case of rate limiting a client it send a HTTP status code 429 Too
Many Requests and a header to the response, which shows the maximum
request per hour (based on RFC 6585):

     X-Rate-Limit: 6000

Registry

The active rate limiters are stored in a registry. They are created
based on routes or command line flags. The registry synchronizes
access to the shared rate limiters. A registry has default settings it
will apply and it will use the disable rate limiter in case it's not
defined in the configuration or not global enabled.

*/

package ratelimit
