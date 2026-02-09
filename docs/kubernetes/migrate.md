This provides a guide for people that want to migrate from another Ingress Controller to Skipper.

You can use our [Skipper install guide](ingress-controller.md) to
deploy skipper and test it. Different Ingress controllers have a lot
of advantages and disadvantages. Skipper is the most feature rich
controller for HTTP. Skipper does not support lower level protocols as
plain TCP or plain TLS routes. Skipper can be run as controller or you
can build your own controller, because the implementation is a library
first code base. You can also check our own [production configuration](https://github.com/zalando-incubator/kubernetes-on-aws/tree/dev/cluster/manifests/skipper).

Skipper is very good at serving HTTP APIs, it's much faster than
others (like nginx or envoy) in routing. Its routing tree is capabable
of efficient routing up to 500k routes (not a kubernetes controller
installation). In comparision the gateway-api tests end at 5k
routes. We run ourselves Skipper as ingress controller in clusters
with many above 15k routes. Our routing capabilities is based on
[predicates](../reference/predicates.md),
which match the best route for a given request.

Skipper can modify every detail in HTTP request and response by using
[filters](../reference/filters.md), that you apply to a route. One more complex
example is authentication for example via [Open Policy Agent](../reference/filters.md#open-policy-agent)
or [cluster based rate limits](../reference/filters.md#clusterclientratelimit).

Skipper has also a very good visibility features, Prometheus metrics,
access logs in Apache format, Opentracing/OTel with detailed proxy
spans that show slowness in detail. Check our [operations
guide](../operation/operation.md) to see what you get.

Skipper is built to support regular config changes by using
dataclients to feed a rebuilt of the routing tree. We rebuild the
routing tree every 3s, so config changes through Kubernetes Ingress
are quasi instant.

## Ingress Nginx

[Ingress Nginx](https://kubernetes.github.io/ingress-nginx) is the
most used Ingress Controller as of today. It was early available,
OpenSource and maintained by the community. It is trusted, because
there is no company that would change to some proprietary license to
get some money and Nginx is a well-known trusted and efficient HTTP
proxy. The [bad news](https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/)
are that it will be retired soon, because of lack on support on
maintainers.

### Pros and Cons Nginx <-> Skipper

Missing features from Nginx:

- TLS routing
- TCP routing
- UDP routing

Nginx is very efficient in streaming data. If you need to stream tons
of data, then you likely want to check some other controller, because
skipper is not made for heavy data streaming. So if you serve 100GB
pictures through ingress, check haproxy or similar proxies.

If you serve mostly HTTP APIs, skipper provides you a solid solution,
that shines with efficiency, visibility, routing with predicates and
filters. Many users build their own custom proxy based on skipper.
Skipper was used since 10 years in production as an Ingress
Controller at [Zalando](https://www.zalando.com).

### Migration by feature

Ingress Nginx uses a lot of annotations and every feature has a lot of
knobs that you need to configure via annotations.
You can not just use the same annotations!

Skipper has [11 annotations](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/#skipper-ingress-annotations),
the most used one is `zalando.org/skipper-filter`.  Skipper uses the
composite pattern and the UNIX philosophy: every filter should do only
one job and it should do it well. You will combine filters to make
things work as you want. This is a very powerful option, which you
likely know from a shell!

Examples:

#### Mirror / tee

Nginx mirror
```
nginx.ingress.kubernetes.io/mirror-target: https://1.2.3.4$request_uri
nginx.ingress.kubernetes.io/mirror-host: "test.env.com"
```

Skipper [tee filters](../reference/filters.md#shadow-traffic)
```
zalando.org/skipper-filter: tee("https://test.env.com")
```

#### Allow listing

Nginx
```
nginx.ingress.kubernetes.io/whitelist-source-range: 10.0.0.0/24,172.10.0.1
```

Skipper predicate [ClientIP](../reference/predicates.md#clientip) or [Source](../reference/predicates.md#source)
```
zalando.org/skipper-predicate: ClientIP("10.0.0.0/24", "172.10.0.1")
```

#### Rate limit

Nginx (supports only by pod limit)
```
nginx.ingress.kubernetes.io/limit-rps: 100
```

Skipper supports different style of [rate limit filters](../reference/filters.md#rate-limit). Some filters also support [template variables](../reference/filters.md#template-placeholders).
```
# by pod limit
zalando.org/skipper-filter: clientRatelimit(100, "1s")

# by "group" key for the whole cluster
zalando.org/skipper-filter: clusterClientRatelimit("groupA", 100, "1s")

# by "group" key for the whole cluster by Authorization header
zalando.org/skipper-filter: clusterClientRatelimit("groupB", 100, "1m", "Authorization")

# allow 10 requests per minute for each unique PHPSESSID cookie with bursts of up to 5 requests
clusterLeakyBucketRatelimit("session-${request.cookie.PHPSESSID}", 10, "1m", 5, 1)
```

#### Redirect

Nginx
```
nginx.ingress.kubernetes.io/permanent-redirect: https://example.org
nginx.ingress.kubernetes.io/permanent-redirect-code: '308'
```

Skipper custom routes supports eskip, skipper's routing language.  The
[<shunt> backend](../reference/backends.md#shunt-backend) responds to the client
directly from the proxy.
```
zalando.org/skipper-routes: |
  r: True() -> redirectTo(308, "https://example.org") -> <shunt>;
```

#### CORS

Nginx
```
nginx.ingress.kubernetes.io/enable-cors: "true"
nginx.ingress.kubernetes.io/cors-allow-origin: "https://api.example.org, https://www.example.org"
nginx.ingress.kubernetes.io/cors-allow-credentials: "true"
```

skipper
```
// annotation for the normal ingress
zalando.org/skipper-filter: |
  corsOrigin("https://api.example.org", "https://www.example.org")
  -> setResponseHeader("Access-Control-Allow-Methods", "GET, OPTIONS")
  -> setResponseHeader("Access-Control-Allow-Headers", "Authorization")

// route for the OPTIONS request
zalando.org/skipper-routes: |
  Method("OPTIONS")
  -> corsOrigin("https://api.example.org", "https://www.example.org")
  -> setResponseHeader("Access-Control-Allow-Methods", "GET, OPTIONS")
  -> setResponseHeader("Access-Control-Allow-Headers", "Authorization")
  -> status(200) -> <shunt>
```
