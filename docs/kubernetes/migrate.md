This provides a guide for people that want to migrate from another Ingress Controller to Skipper.

## Skipper

### Why Skipper as Ingress controller?

Skipper is stable software that has quite impressive features as a [modern http router](https://www.usenix.org/conference/lisa18/presentation/szucs).
Skipper is a library first implementation of an http proxy written in Go.
Go is the infrastructure computer language used by Kubernetes, Containerd, Docker, Prometheus, and etc.
Learn one computer language and you are able to customize skipper for your needs.

### Does Skipper scale?

Skipper is used as core infrastructure by Zalando SE, a >10B/y GMV
German DAX company, from Europe with headquarters in Berlin and
locations in different countries across Europe. The scale is similar
to shopify or lyft or other big companies.

We run 500k-7M rps through the ingress data plane every day. There is
no known limit other than node capacity and load balancer member
limits (example AWS has TG member limits for each AZ).

Skipper itself scales linear by the number of CPUs and can run
with sub-millisecond overhead. Of course it depends on route
configurations, so features you put into a route, your autoscaling
configuration and load patterns.

You can check yourself how we configure Skipper as Ingress controller
in our [production configuration](https://github.com/zalando-incubator/kubernetes-on-aws/tree/dev/cluster/manifests/skipper).
We use a 2-layer load balancer deployment with AWS Network Load
Balancer and Skipper. AWS Network Load Balancers (NLB) are shared and
created by [kube-ingress-aws-controller](https://github.com/zalando-incubator/kube-ingress-aws-controller).
DNS Names pointing to NLBs are managed by [External-DNS](https://github.com/kubernetes-sigs/external-dns).

Skipper has been run with more than 800000 routes. This was of course not as
a Kubernetes Ingress controller. Contributors to skipper are known to
run skipper with about 400000 routes. We run skipper as Kubernetes
Ingress controller with more than 20000 routes in production and
tested with up to 40000 routing objects successfully. On the other
hand the Kubernetes gateway-api tests scalability with a maximum of
5000 routing objects.

### How do you achieve safety?

Runtime safety is achieved by operational excellence. We have
dedicated documentation to explain every aspect of it in our
[operations guide](../operation/operation.md).

Another part of safety is actually the developer that creates routing
objects like Ingress or RouteGroups. If these have errors, it can lead
to an outage of an application. At Zalando we have around 350 teams
that deploy routing objects and applications every day. We observe all
kinds of errors and we are able to make it very hard to make errors by
leveraging Kubernetes [validation webhook for Ingress and RouteGroup](../kubernetes/routegroup-validation.md).

Skipper has a very good route matching feature set by leveraging a
tree search to reduce routes to be scanned for a match. After that,
the skipper matches by number of [predicates](../reference/predicates.md) the best
route. Understanding the [route matching algorithm](../reference/architecture.md#route-matching)
makes sense if you configure complex routes. It’s not uncommon that
people have Kubernetes routing objects with 20-100 routes for one
application.

### Skipper deployment

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

### How do I map Ingress-NGINX features to skipper Ingress features?

Ingress Nginx uses a lot of annotations.

Skipper has only a couple of annotations to support a similar set of
features. Skipper uses composite patterns and you can test all routing
features on your local machine without running a Kubernetes cluster.
Skipper’s routing language is “[eskip](https://pkg.go.dev/github.com/zalando/skipper/eskip)”,
which is focused on http routing.

Let's see an eskip example, which has 2 routes: r1 and r2.

* R1 matches the host header to www.zalando.de and the path prefix is
/api. Then it will execute a path modification to remove /api to the
outgoing request that will be sent to https://internal.loadbalancer.example.
* R2 matches the host header to www.zalando.de for all other paths and
  use the load balancer algorithm powerOfRandomNChoices to proxy to
  the listed backend endpoints.

```
r1: Host(“www.zalando.de”) && PathSubtree(“/api”)
	-> modPath(“/api/(.*)”, “/$1”)
	-> “https://internal.loadbalancer.example”;
r2: Host(/^www[.]zalando[.]de(:[0-9]+)?$/)
	-> <powerOfRandomNChoices, "http://10.0.0.1:8080", "http://10.0.5.23:8080">;
```

Eskip Syntax is simple, but powerful by composition:

```
RouteID1: predicate1 && … && predicateN
	-> filter1
	-> ..
	-> filterM
	-> <backend>;
```

You can see that there is no logical OR. If you need an
“OR”, you just create another route!

Ingress example with predicates and filters

```
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-predicate: predicate1 && .. && predicateN
    zalando.org/skipper-filter: filter1 -> .. -> filterM
  name: my-app
spec:
  rules:
  - host: my-app.example.org
    http:
      paths:
      - backend:
          service:
            name: app-svc
            port:
              number: 8080
```

The Skipper native CRD is called
[RouteGroup](./routegroup-crd.md)
and allows better control of complex routes than Ingress. You can
create multiple routes with one RouteGroup.  The following example
shows how to route:

1. Requests with paths other than `/api` will be proxied to Kubernetes
service type ClusterIP `my-service` with port `8080` by load balancer
algorithm powerOfRandomNChoices (skipper will use Kubernetes endpoints
or endpointslices depending on the configuration)
2. Redirect requests with paths other than `/api` that have http
header X-Forwarded-Proto with value "http" to the same URL but via
https
3. Requests with path prefix `/api` will be modified from `/api` to
`/` and proxied to your Kubernetes service type ClusterIP `my-service`
with port `8080` by load balancer algorithm powerOfRandomNChoices

```
kind: RouteGroup
metadata:
  name: my-route-group
spec:
  backends:
  - name: my-backend
    type: service
    serviceName: my-service
    servicePort: 8080
    algorithm: powerOfRandomNChoices
  - name: redirectShunt
    type: shunt
  defaultBackends:
  - backendName: my-backend
  hosts:
  - api.example.org
  - legacy-name.example.org
  routes:
  - pathSubtree: /
  - pathSubtree: /
    predicates:
    - Header("X-Forwarded-Proto", "http")
    filters:
    - redirectTo(302, "https:")
    backends:
    - backendName: redirectShunt
  - pathSubtree: /api
    filters:
    - modPath("^/api/(.*)/v2$", "/$1")
```

As you can see if you follow the RouteGroup example carefully, skipper
routes by path first. Check out the
[route matching algorithm](../reference/architecture.md#route-matching)
explained in our documentation.

#### Rewrite path

Skipper filters can modify the request and the response.
You can use [HTTP path filters](../reference/filters.md#http-path) to rewrite the request paths.
Example: To rewrite the request path `/api/*` to `/*` use [modPath](../reference/filters.md#modpath) filter:

```
modPath("/api/(.*)", "/$1")
```

#### Redirect - change the base URL and path

We match the path prefix `/a/base/` and want to redirect to
`https://another-example.com/my/new/base/` such that requests for
example to `/a/base/products/5` will be redirected to
`https://another-example.com/my/new/base/products/5`, you can create a
route which will responded by skipper directly
( [<shunt> backend](../reference/backends.md#shunt-backend) )
with a redirect with Location header set to `another-example.com` and
status code 308:

```
redirect: PathSubtree("/a/base/")
          -> modPath("/a/base/", "/my/new/base/")
          -> redirectTo(308, "https://another-example.com")
          -> <shunt>'
```

Same by an Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: |
      modPath("/a/base/", "/my/new/base/") -> redirectTo(308, "https://another-example.com")
  name: my-app
spec:
  rules:
  - host: my-app.example.org
    http:
      paths:
      - backend:
          service:
            name: app-svc
            port:
              number: 8080
        path: /a/base/
        pathType: Prefix
```

Same by a Routegroup

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-route-group
spec:
  backends:
  - name: redirectShunt
    type: shunt
  routes:
  - pathSubtree: /a/base
    filters:
    - modPath("/a/base/", "/my/new/base/")
    - redirectTo(308, "https://another-example.com")
    backends:
    - backendName: redirectShunt
```

#### HTTP Header modifier

Skipper has a bunch of header specific filters.  In general you can
`set`, `mod` (modify), `append`, `copy` or `drop` request and response
headers.

Example modifies the request Host header by [modRequestHeader](../reference/filters.md#modrequestheader)
to change `zalando.TLD` to `www.zalando.TLD` and redirect modified permanently by 301 status
code:

```
enforce_www: *
	-> modRequestHeader("Host", "^zalando\.(\w+)$", "www.zalando.$1")
	-> redirectTo(301)
	-> <shunt>;
```

If you want to preserve the Host header if you proxy requests to your
backends, you can use a flag to Skipper to set the default
`-proxy-preserve-host=true` (default is false, but we recommend in
Kubernetes to set it to true). You can use the filter
`preserveHost("false")` to set it back to false on each route you want
to differ from the chosen default.

```
preserveHost("true")
```

You can automatically set
[CORS headers correctly by host specifications](../reference/filters.md#corsorigin),
Example:

```
main_route:
PathSubtree("/")
 -> corsOrigin()
 -> setResponseHeader("Access-Control-Allow-Credentials", "true")
 -> setResponseHeader("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, PATCH, DELETE")
 -> "http://backend.example.org";

preflight_route:
PathSubtree("/") && Method("OPTIONS")
 -> corsOrigin()
 -> setResponseHeader("Access-Control-Allow-Credentials", "true")
 -> setResponseHeader("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, PATCH, DELETE")
 -> setResponseHeader("Access-Control-Allow-Headers", "authorization, origin, content-type, accept")
 -> status(200)
 -> <shunt>;
```

There are a bunch of things more you can do with
[skipper on HTTP headers](../reference/filters.md#http-headers),
for example change encoding, copy headers to the URL query or
set XFF headers similar to either Nginx or AWS ALB.  If you miss
anything please file an [issue in our bug tracker](https://github.com/zalando/skipper/issues/new/choose).
It’s often not much work to add such features.

#### Blue-Green deployment

A very common deployment configuration for your applications is to
switch traffic slowly by some percentage and observe if your metrics
like error rates or latency percentiles are fine.  By choosing skipper
you can use Kubernetes Ingress or RouteGroups to achieve this and at
Zalando we use [stackset-controller](https://github.com/zalando-incubator/stackset-controller)
to deploy most applications that need such a deployment strategy.
Blue-Green deployment to set traffic to 10% for "green" and 90% for
"blue". You can have more than 2 backends (rainbow deployment) and
config values are weights and not percentage so setting 1000 and 1 is
fine.

Eskip by using [TrafficSegment](../reference/predicates.md#trafficsegment) or [Traffic](../reference/predicates.md#traffic) predicates:

```
// TrafficSegment
green: TrafficSegment(0.0, 0.1)
blue:  TrafficSegment(0.1, 1.0)

// Traffic
green: Traffic(0.1)
blue:  *
```

Ingress:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    zalando.org/backend-weights: |
      {"app-svc-green": 10, "app-svc-blue": 90}
  name: my-app
spec:
  rules:
  - host: my-app.example.org
    http:
      paths:
      - backend:
          service:
            name: app-svc-blue
            port:
              number: 8080
      - backend:
          service:
            name: app-svc-green
            port:
              number: 8080
```

RouteGroup:

```yaml
kind: RouteGroup
metadata:
  name: my-route-group
spec:
  backends:
  - name: blue
    type: service
    serviceName: app-svc-blue
    servicePort: 8080
  - name: green
    type: service
    serviceName: app-svc-green
    servicePort: 8080
  defaultBackends:
  - backendName: app-svc-blue
    weight: 90
  - backendName: app-svc-green
    weight: 10
  hosts:
  - my-app.example.org
  routes:
  - pathSubtree: /
```

#### Shadow Traffic aka Traffic Mirror

Requests will be copied in an efficient way, such that you can test a
new application with current production traffic. There are simple
configurations that allow you to duplicate all traffic to another
application and you can also achieve
[weighted shadow traffic](../tutorials/shadow-traffic.md) explained
in our documentation.  The response of the shadow backend will be
dropped at the proxy level

Eskip: By 10% chance, split the traffic by "tee" and loopback the copy
through the routing tree, which will select the "shadow" route for the
copied request. The `True()` predicate is used to dominate the weights
of the routes by the number of predicates. If you don’t understand the
last sentence please read the [route matching](../reference/architecture.md#route-matching)
documentation.

```
main: * -> "https://main.example.org";
split: Traffic(.1) -> teeLoopback("shadow-test-1") -> "https://main.example.org";
shadow: Tee("shadow-test-1") && True() -> "https://shadow.example.org";
```

Ingress: If you want to achieve the same shadow traffic with weights,
you need to either use 3 Ingress objects or use
`zalando.org/skipper-routes` annotation. We recommend using RouteGroup
instead for such complex routes. We show here only a 100% shadow
traffic in Ingress configuration for your own safety:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    zalando.org/filter: tee("https://shadow.example.org")
  name: my-app
spec:
  rules:
  - host: main.example.org
    http:
      paths:
      - backend:
          service:
            name: app-svc
            port:
              number: 8080
```

RouteGroup is similar to the eskip example with 3 routes and weighted
shadow traffic, such that 10% of the requests will be copied to the
shadow traffic backend.

```yaml
kind: RouteGroup
metadata:
  name: my-route-group
spec:
  backends:
  - name: main
    type: service
    serviceName: app-svc
    servicePort: 8080
  - name: shadow
    type: service
    serviceName: app-svc-new
    servicePort: 8080
  defaultBackends:
  - backendName: main
  hosts:
  - main.example.org
  routes:
  - pathSubtree: /
  - pathSubtree: /
    predicates:
    - Traffic(.1)
    filters:
    - teeLoopback("shadow-test-1")
  - pathSubtree: /
    predicates:
    - Tee("shadow-test-1")
    - True()
    backends:
    - backendName: shadow
```

#### Matching HTTP requests by Query, Method, Cookie and more

Matching HTTP requests by [Content-Length](../reference/predicates.md#contentlengthbetween),
[Query](../reference/predicates.md#queryparam), API Key,
[JWT data](../reference/predicates.md#auth),
[Methods](../reference/predicates.md#methods), [Cookie](../reference/predicates.md#cookie),
time or by OTel data is all possible by using
[predicates](../reference/predicates.md).

For example many times you need to do quirks and support for example a
shared secret (API Key). You do not want to specify the secret in a
Kubernetes object nor in plain text in your code repository and you
want to rotate your shared secret?
Use [HeaderSHA256](../reference/predicates.md#headersha256) predicate!


#### Modify HTTP query

You can [strip](../reference/filters.md#stripquery), [set](../reference/filters.md#setquery), [drop](../reference/filters.md#dropquery) the query or [copy a query to a header](../reference/filters.md#querytoheader).

#### Protect backend applications from security vulnerabilities

Skipper has some outstanding capabilities that let you block traffic
based on request body data. For example if you remember
[log4shell](https://en.wikipedia.org/wiki/Log4Shell), your CDN and
security provider will likely fix it for you but you can use skipper
filters [blockContent](../reference/filters.md#blockcontent) and
[blockContentHex](../reference/filters.md#blockcontenthex) to protect
routes. In combination with default filters `-default-filters-prepend`
you can block content streamed through the skipper proxy and block the
request reaching your backend. Applying this protection did not show
up in any kind of cost increase, because the efficiency of streaming.

```
blockContent("Malicious Content", "${")
blockContentHex("deadbeef", "000a")
```

#### Authentication

Skipper supports a wide range of
[authentication and authorization mechanisms](../reference/filters.md#authentication-and-authorization)

like Basic Auth, Webhook, JWT, Tokeninfo, Tokenintrospection, OAuth2
authorization code grant flow, OpenID Connect or AWS Sigv4. We also
have first class support for [Open Policy Agent](https://www.openpolicyagent.org/) (OPA) integrated into
skipper. We do not want to run OPA as a sidecar, because of the
overhead it creates to have webhook HTTP requests integrations. Please
check out our [Authnz filters](../reference/filters.md#authentication-and-authorization) for more detailed information.

#### Rate Limits

If you have an ingress data plane that is scaled by
HorizonalPodAutoscaling (hpa), you want to have rate limit
configuration that automatically adapts no matter if you run 2 or 100
proxy pods.  Skipper has several filters that can achieve this, in
case you have configured skipper to use Redis or Valkey as scalable
ring shard storage for rate limit buckets.

Time window based rate limit filters:

- [clusterRatelimit](../reference/filters.md#clusterratelimit) limits
  all requests of the group of routes
- [clusterClientRatelimit](../reference/filters.md#clusterclientratelimit)
  limits all requests of the same client of the specified group of
  routes

Leaky bucket rate limit filter:

- [clusterLeakyBucketRatelimit](../reference/filters.md#clusterleakybucketratelimit)

Load shedding:

- [admissionControl](../reference/filters.md#admissioncontrol)

#### Logs

You need to enable/disable logs based on status codes for debugging,
to reduce costs or you need to mask secrets from logs. You can use
skipper [log filters](../reference/filters.md#logs) doing that.

#### Load Balancer Algorithm config

Skipper supports ingress annotation `zalando.org/skipper-loadbalancer`
to choose a different [load
balancer](../reference/backends.md#load-balancer-backend) algorithm
other than the default. The default you can set by
`-kubernetes-default-lb-algorithm` flag to skipper.

Available algorithms:

- `roundRobin`
- `random`
- `consistentHash`
- `powerOfRandomNChoices`

Your JIT based runtime applications have to ramp up slowly to traffic.
You can use the [fadeIn](../reference/filters.md#fadein) filter to
configure the traffic ramp up for new pods.

Special applications have special needs. For example we have an
application that uses consistentHash load balancer algorithm to have a
very good cache hit rate. Sometimes hot partitions have so much
pressure that you need to automatically spill over to serve the shard
by more pods. A combination of
[consistentHashKey](../reference/filters.md#consistenthashkey)
specifying an HTTP header and
[consistentHashBalanceFactor](../reference/filters.md#consistenthashbalancefactor)
filters with algorithm consistentHash can do this.

Ingress example

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-loadbalancer: consistentHash
  name: app
spec:
  rules:
  - host: app.example.org
    http:
      paths:
      - backend:
          service:
            name: app-svc
            port:
              number: 80
        pathType: ImplementationSpecific
```

RouteGroup example

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-route-group
spec:
  backends:
  - name: my-backend
    type: service
    serviceName: my-service
    servicePort: 80
    algorithm: consistentHash
  defaultBackends:
  - backendName: my-backend
  routes:
  - path: /products/:productId
    filters:
    - fadeIn("3m", 1.5)
    - consistentHashKey("${productId}")
    - consistentHashBalanceFactor(1.25)
```

#### Timeouts

The operator of the Skipper Ingress controller can set timeout
boundaries to achieve safety.  Skipper supports
[timeout](../reference/filters.md#timeout) filters to set
backendTimeout, readTimeout and writeTimeout. While read and write
timeouts are limiting the time to stream the http body, the backend
timeout measures the full request-response roundtrip from skipper to
the backend.

As operator you can control timeouts on the server handler and to the
backend using flags or config:

```
  -expect-continue-timeout-backend duration
  -response-header-timeout-backend duration
  -timeout-backend duration
  -tls-timeout-backend duration
  -idle-timeout-server duration
  -read-header-timeout-server duration
  -read-timeout-server duration
  -write-timeout-server duration
```

See also [connection options](../operation/operation.md#connection-options)
in our operations guide.

#### CORS

In general CORS handling requires 2 routes. You need to handle the
preflight OPTIONS request and the real request that you want to proxy
to the application.  Skipper has a [corsOrigin](../reference/filters.md#corsorigin)
filter that dynamically sets the Origin header based on the incoming
request and the chosen allow list passed to the filter.

Eskip example

```
main_route:
PathSubtree("/")
 -> corsOrigin("https://www.example.org", "https://api.example.org")
 -> setResponseHeader("Access-Control-Allow-Credentials", "true")
 -> setResponseHeader("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, PATCH, DELETE")
 -> "http://backend.example.org";

preflight_route:
PathSubtree("/") && Method("OPTIONS")
 -> corsOrigin("https://www.example.org", "https://api.example.org")
 -> setResponseHeader("Access-Control-Allow-Credentials", "true")
 -> setResponseHeader("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, PATCH, DELETE")
 -> setResponseHeader("Access-Control-Allow-Headers", "authorization, origin, content-type, accept")
 -> status(200)
 -> <shunt>;
```

RouteGroup example

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-route-group
spec:
  backends:
  - name: my-shunt
    type: shunt
  - name: my-backend
    type: service
    serviceName: my-service
    servicePort: 80
  defaultBackends:
  - backendName: my-backend
  hosts:
  - www.example.org
  - api.example.org
  routes:
  - pathSubtree: /
    filters:
    - corsOrigin("https://www.example.org", "https://api.example.org")
    - setResponseHeader("Access-Control-Allow-Credentials", "true")
    - setResponseHeader("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, PATCH, DELETE")
  - pathSubtree: /
    methods:
    - OPTIONS
    filters:
    - corsOrigin("https://www.example.org", "https://api.example.org")
    - setResponseHeader("Access-Control-Allow-Credentials", "true")
    - setResponseHeader("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, PATCH, DELETE")
    - setResponseHeader("Access-Control-Allow-Headers", "authorization, origin, content-type, accept")
    backends:
    - backendName: my-shunt
```

#### Backend Protocol

Skipper supports HTTP, HTTPS and FastCGI.
Websockets are supported by HTTP upgrade headers that do not need any
kind of configuration other than the operator enabling this feature.

Ingress example

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-backend-protocol: fastcgi
  name: app
spec:
  rules:
  - host: app.example.org
    http:
      paths:
      - backend:
          service:
            name: app-svc
            port:
              number: 80
        pathType: ImplementationSpecific
```

For cross cluster migrations skipper also supports Ingress annotation
`zalando.org/skipper-backend: forward` and RouteGroup `type: forward`.


### Migration by feature

Ingress Nginx uses a lot of annotations and every feature has a lot of
knobs that you need to configure via annotations.
You can not just use the same annotations!

Skipper has [11 annotations](./ingress-usage.md#skipper-ingress-annotations),
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

## We're Here to Help

If you have any kind of question or ideas regarding skipper, please
feel free to contact us.  You can reach us in
[Gophers Slack community channel #skipper](https://gophers.slack.com/archives/C82Q5JNH5).

You can also create issues in our [Github repository](https://github.com/zalando/skipper/issues).

We do not offer paid support, but we are happy to answer your
questions or discuss your ideas.
