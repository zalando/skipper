## Overview

Ratelimits are calculated for a number of requests and a
`time.Duration` for a given bucket.

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

The same for Authorization Header you would use:

```
clientRatelimit(10, "1m", "auth")
```

Internally skipper has a clean interval to clean up old buckets to reduce
the memory footprint in the long run.

## Cluster Ratelimit

A cluster ratelimit computes all requests for all skipper peers. This
requires, that you run skipper with `-enable-swarm` and all
requirements, that are dependent on your dataclient in use.

In case of Kubernetes you might specify additionally
`-swarm-label-selector-key`, which defaults to "application" and
`-swarm-label-selector-value`, which defaults to "skipper-ingress" and
`-swarm-namespace`, which defaults to "kube-system".

The following shows the setup of a cluster ratelimit:

![Picture showing Skipper swarm and ratelimit](/skipper/img/swarm-and-cluster-ratelimit.svg)

### Backend Ratelimit

The backend ratelimit filter is `clusterRatelimit()`. You can define
how many requests a route allows for a given `time.Duration` in total
for all skipper instances summed up.

For example to limit the route to 10 requests per minute in total for
the cluster, you can specify:

```
clusterRatelimit(10, "1m")
```

### Client Ratelimit

The client ratelimit filter is `clusterClientRatelimit()` and it uses
information from the request to find the bucket which will get the
increased request count.  You can define how many requests a client is
allowed to hit this route for a given `time.Duration` in total for all
skipper instances summed up.

For example to limit the route to 10 requests per minute for the full
skipper swarm for the same client selected by the X-Forwarded-For
header, you can specify:

```
clusterClientRatelimit(10, "1m")
```

The same for Authorization Header you would use:

```
clusterClientRatelimit(10, "1m", "auth")
```

Internally skipper has a clean interval to clean up old buckets to reduce
the memory footprint in the long run.
