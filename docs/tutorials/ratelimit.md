## Overview

Ratelimits are calculated for a number of requests and a
`time.Duration` for a given bucket. To enable rate limits you need to
run skipper with `-enable-ratelimits`.

A `time.Duration` is specified as string and can for example be "10s"
for ten seconds, "5m" for five minutes or "2h" for two hours.

As bucket skipper can use either the backend or some client
information.

In case of a backend ratelimit the bucket is only one global for one
route.

In case of a client ratelimit the buckets are created by the
used `ratelimit.Lookuper`, which defaults to the `X-Forwarded-For`
header, but can be also the `Authorization` header. So for the client
ratelimit with `X-Forwarded-For` header, the client IP that the first
proxy in the list sees will be used to lookup the bucket to count
requests.

## Instance local Ratelimit

Filters `ratelimit()` and `clientRatelimit()` calculate the ratelimit
in a local view having no information about other skipper instances.

### Backend Ratelimit

The backend ratelimit filter is `ratelimit()` and it is the simplest
one. You can define how many requests a route allows for a given
`time.Duration`.

For example to limit the route to 10 requests per minute for each
 skipper instance, you can specify:

```
ratelimit(10, "1m")
```

### Client Ratelimit

The client ratelimit filter is `clientRatelimit()` and it uses
information from the request to find the bucket which will get the
increased request count.

For example to limit the route to 10 requests per minute for each
skipper instance for the same client selected by the X-Forwarded-For
header, you can specify:

```
clientRatelimit(10, "1m")
```

There is an optional third argument that selects the same client by HTTP
header value. As an example for Authorization Header you would use:

```
clientRatelimit(10, "1m", "Authorization")
```

The optional third argument can create an AND combined Header
ratelimit. The header names must be separated by `,`. For example all of the
specified headers have to be the same to recognize them as the same
client:

```
clientRatelimit(10, "1m", "X-Forwarded-For,Authorization,X-Foo")
```

Internally skipper has a clean interval to clean up old buckets to reduce
the memory footprint in the long run.

## Cluster Ratelimit

A cluster ratelimit computes all requests for all skipper peers. This
requires, that you run skipper with `-enable-swarm` and select one of
the two implementations:

- [Redis](https://redis.io)
- [SWIM](https://www.cs.cornell.edu/projects/Quicksilver/public_pdfs/SWIM.pdf)

Make sure all requirements, that are dependent on the implementation
and your dataclient in use.

### Redis based Cluster Ratelimits

This solution is independent of the dataclient being used.  You have
to run N number of [Redis](https://redis.io) instances, where N is >
0.  Specify `-swarm-redis-urls`, multiple instances can be separated
by `,`, for example: `-swarm-redis-urls=redis1:6379,redis2:6379`. For
running skipper in Kubernetes with this, see also [Running with
Redis based Cluster Ratelimits](../../kubernetes/ingress-controller/#redis-based)

The implementation use [redis ring](https://godoc.org/github.com/go-redis/redis#Ring)
to be able to shard via client hashing and spread the load across
multiple Redis instances to be able to scale out the shared storage.

The ratelimit algorithm is a sliding window and makes use of the
following Redis commands:

- [ZREMRANGEBYSCORE](https://redis.io/commands/zremrangebyscore),
- [ZCARD](https://redis.io/commands/zcard),
- [ZADD](https://redis.io/commands/zadd) and
- [ZRANGEBYSCORE](https://redis.io/commands/zrangebyscore)

![Picture showing Skipper with Redis based swarm and ratelimit](../img/redis-and-cluster-ratelimit.svg)

### SWIM based Cluster Ratelimits

[SWIM](https://www.cs.cornell.edu/projects/Quicksilver/public_pdfs/SWIM.pdf)
is a "Scalable Weakly-consistent Infection-style Process Group
Membership Protocol", which is very interesting to use for cluster
ratelimits. The implementation has some weaknesses in the algorithm,
that lead sometimes to too much ratelimits or too few and therefore is
not considered to be stable. For running skipper in Kubernetes with
this, see also [Running with SWIM based Cluster Ratelimits](../../kubernetes/ingress-controller/#swim-based)

In case of Kubernetes you might specify additionally
`-swarm-label-selector-key`, which defaults to "application" and
`-swarm-label-selector-value`, which defaults to "skipper-ingress" and
`-swarm-namespace`, which defaults to "kube-system".

The following shows the setup of a SWIM based cluster ratelimit:

![Picture showing Skipper SWIM based swarm and ratelimit](../img/swarm-and-cluster-ratelimit.svg)

### Backend Ratelimit

The backend ratelimit filter is `clusterRatelimit()`. You can define
how many requests a route allows for a given `time.Duration` in total
for all skipper instances summed up. The first parameter is the group
parameter, which can be used to select the same ratelimit group across
one or more routes

For example rate limit "groupA" limits the rate limit group to 10
requests per minute in total for the cluster, you can specify:

```
clusterRatelimit("groupA", 10, "1m")
```

### Client Ratelimit

The client ratelimit filter is `clusterClientRatelimit()` and it uses
information from the request to find the bucket which will get the
increased request count.  You can define how many requests a client is
allowed to hit this route for a given `time.Duration` in total for all
skipper instances summed up. The first parameter is the group
parameter, which can be used to select the same ratelimit group across
one or more routes

For example rate limit "groupB" limits the rate limit group to 10
requests per minute for the full skipper swarm for the same client
selected by the X-Forwarded-For header, you can specify:

```
clusterClientRatelimit("groupB", 10, "1m")
```

The same for Authorization Header you would use:

```
clusterClientRatelimit("groupC", 10, "1m", "Authorization)
```

The optional fourth argument can create an AND combined Header
ratelimit. The header names must be separated by `,`. For example all
of the specified headers have to be the same to recognize them as the
same client:

```
clusterClientRatelimit("groupC", 5, "10s", "X-Forwarded-For,Authorization,X-Foo")
```

Internally skipper has a clean interval to clean up old buckets to reduce
the memory footprint in the long run.
