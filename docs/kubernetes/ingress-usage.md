# Skipper Ingress Usage

This documentation is meant for people deploying to Kubernetes
Clusters and describes to use Ingress and low level and high level
features Skipper provides

## Skipper Ingress Annotations

Annotation | example data | usage
--- | --- | ---
zalando.org/backend-weights | `{"my-app-1": 80, "my-app-2": 20}` | blue-green deployments
zalando.org/skipper-filter | `consecutiveBreaker(15)` | arbitrary filters
zalando.org/skipper-predicate | `QueryParam("version", "^alpha$")` | arbitrary predicates
zalando.org/skipper-routes | `Method("OPTIONS") -> status(200) -> <shunt>` | extra custom routes
zalando.org/ratelimit | `ratelimit(50, "1m")` | deprecated, use zalando.org/skipper-filter instead
zalando.org/skipper-ingress-redirect | `"true"` | change the default HTTPS redirect behavior for specific ingresses (true/false)
zalando.org/skipper-ingress-redirect-code | `301` | change the default HTTPS redirect code for specific ingresses
zalando.org/skipper-loadbalancer | `consistentHash` | defaults to `roundRobin`, [see available choices](../../reference/backends/#load-balancer-backend)

## Supported Service types

Ingress backend definitions are services, which have different
[service types](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services---service-types).

Service type | supported | workaround
--- | --- | ---
ClusterIP | yes | ---
NodePort | yes | ---
ExternalName | no, [related issue](https://github.com/zalando/skipper/issues/549) | [use deployment with routestring](../data-clients/route-string.md#proxy-to-a-given-url)
LoadBalancer | no | it should not, because Kubernetes cloud-controller-manager will maintain it


## HTTP Host header routing

HTTP host header is defined within the rules `host` section and this
route will match by http `Host: app-default.example.org` and route to
endpoints selected by the Kubernetes service `app-svc` on port `80`.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

To have 2 routes with different `Host` headers serving the same
backends, you have to specify 2 entries in the rules section, as
Kubernetes defined the ingress spec. This is often used in cases of
migrations from one domain to another one or migrations to or from
bare metal datacenters to cloud providers or inter cloud or intra
cloud providers migrations. Examples are AWS account migration, AWS to
GCP migration, GCP to bare metal migration or bare metal to Alibaba
Cloud migration.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80
      - host: foo.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

### Multiple Ingresses defining the same route

!!! Warning

    If multiple ingresses define the same host and the same predicates, traffic routing may become non-deterministic.

Consider the following two ingresses which have the same hostname and therefore
overlap. In skipper the routing of this is currently undefined as skipper
doesn't pick one over the other, but just creates routes (possible overlapping)
for each of the ingresses.

In this example (taken from the issues we saw in production clusters) one
ingress points to a service with no endpoints and the other to a service with
endpoints. (Most likely service-x was renamed to service-x-live and the old
ingress was forgot).

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
    name: service-x
    spec:
    rules:
    - host: service-x.example.org
        http:
        paths:
        - backend:
            serviceName: service-x # this service has 0 endpoints
            servicePort: 80

&#x200B;

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
    name: service-x-live
    spec:
    rules:
    - host: service-x.example.org
        http:
        paths:
        - backend:
            serviceName: service-x-live
            servicePort: 80

## Ingress path handling

Ingress paths can be interpreted in four different modes:

1. based on the kubernetes ingress specification
2. as plain regular expression
3. as a path prefix

The default is the kubernetes ingress mode. It can be changed by a startup option
to any of the other modes, and the individual ingress rules can also override the
default behavior with the zalando.org/skipper-ingress-path-mode annotation.

E.g.:

    zalando.org/skipper-ingress-path-mode: path-prefix

### Kubernetes ingress specification base path

By default, the ingress path is interpreted as a regular expression with a
mandatory leading "/", and is automatically prepended by a "^" control character,
enforcing that the path has to be at the start of the incoming request path.

### Plain regular expression

When the path mode is set to "path-regexp", the ingress path is interpreted similar
to the default kubernetes ingress specification way, but is not prepended by the "^"
control character.

### Path prefix

When the path mode is set to "path-prefix", the ingress path is not a regular
expression. As an example, "/foo/bar" will match "/foo/bar" or "/foo/bar/baz", but
won't match "/foo/barooz".

When PathPrefix is used, the path matching becomes deterministic when
a request could match more than one ingress routes otherwise.

In PathPrefix mode, when a Path or PathSubtree predicate is set in an
annotation, the predicate in the annotation takes precedence over the normal ingress
path.

## Filters and Predicates

- **Filters** can manipulate http data, which is not possible in the ingress spec.
- **Predicates** change the route matching, beyond normal ingress definitions

This example shows how to add predicates and filters:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-predicate: predicate1 && predicate2 && .. && predicateN
    zalando.org/skipper-filter: filter1 -> filter2 -> .. -> filterN
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

## Custom Routes

Custom routes is a way of extending the default routes configured for
an ingress resource.

Sometimes you just want to return a header, redirect or even static
html content. You can return from skipper without doing a proxy call
to a backend, if you end your filter chain with `<shunt>`. The use of
`<shunt>` recommends the use in combination with `status()` filter, to
not respond with the default http code, which defaults to 404.  To
match your custom route with higher priority than your ingress you
also have to add another predicate, for example the [Method("GET")
predicate](../reference/predicates.md#method) to match the route with higher
priority.

Custom routes specified in ingress will always add the `Host()`
[predicate](../reference/predicates.md#host) to match the host header specified in
the ingress `rules:`. If there is a `path:` definition in your
ingress, then it will be based on the skipper command line parameter
`-kubernetes-path-mode` set one of theses predicates:

- [Path()](../reference/predicates.md#path)
- [PathSubtree()](../reference/predicates.md#pathsubtree)
- [PathRegexp()](../reference/predicates.md#pathregexp)

If you have a `path:` value defined in your ingress resource, a custom
route is not allowed to use `Path()` nor `PathSubtree()` predicates.
You will get an error in Skipper logs, similar to:

```
[APP]time="2019-01-02T13:30:16Z" level=error msg="Failed to add route having 2 path routes: Path(\"/foo/bar\") -> inlineContent(\"custom route\") -> status(200) -> <shunt>"
```

### Redirects

#### Overwrite the current ingress with a redirect

[Sometimes](https://github.com/zalando/skipper/issues/867) you want to
overwrite the current ingress with a redirect to a nicer downtime
page.

The following example shows how to create a temporary redirect with status
code 307 to https://outage.example.org. No requests will pass to your
backend defined, because the created route from the annotation
`zalando.org/skipper-routes` will get 3 Predicates
`Host("^app-default[.]example[.]org$") && Path("/") && PathRegexp("/")`,
instead of the 2 Predicates
`Host("^app-default[.]example[.]org$") && Path("/")`, that will be
created for the ingress backend.

```
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: app
  namespace: default
  annotations:
    zalando.org/skipper-routes: |
       redirect_app_default: PathRegexp("/") -> redirectTo(307, "https://outage.example.org/") -> <shunt>;
spec:
  rules:
  - host: "app-default.example.org"
    http:
      paths:
      - path: /
        backend:
          serviceName: app-svc
          servicePort: 80
```

#### Redirect a specific path from ingress

Sometimes you want to have a redirect from
`http://app-default.example.org/myredirect` to
`https://somewhere.example.org/another/path`.

The following example shows how to create a permanent redirect with status
code 308 from `http://app-default.example.org/myredirect` to
`https://somewhere.example.org/another/path`, other paths will not be
redirected and passed to the backend selected by `serviceName=app-svc` and
`servicePort=80`:

```
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: app
  namespace: default
  annotations:
    zalando.org/skipper-routes: |
       redirect_app_default: PathRegexp("/myredirect") -> redirectTo(308, "https://somewhere.example.org/another/path") -> <shunt>;
spec:
  rules:
  - host: "app-default.example.org"
    http:
      paths:
      - path: /
        backend:
          serviceName: app-svc
          servicePort: 80
```

### Return static content

The following example sets a response header `X: bar`, a response body
`<html><body>hello</body></html>` and respond from the ingress
directly with a HTTP status code 200:

```
zalando.org/skipper-routes: |
  Path("/") -> setResponseHeader("X", "bar") -> inlineContent("<html><body>hello</body></html>") -> status(200) -> <shunt>
```

Keep in mind that you need a valid backend definition to backends
which are available, otherwise Skipper would not accept the entire
route definition from the ingress object for safety reasons.

### CORS example

This example shows how to add a custom route for handling `OPTIONS` requests.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-routes: |
      Method("OPTIONS") ->
      setResponseHeader("Access-Control-Allow-Origin", "*") ->
      setResponseHeader("Access-Control-Allow-Methods", "GET, OPTIONS") ->
      setResponseHeader("Access-Control-Allow-Headers", "Authorization") ->
      status(200) -> <shunt>
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

This will generate a custom route for the ingress which looks like this:

```
Host(/^app-default[.]example[.]org$/) && Method("OPTIONS") ->
  setResponseHeader("Access-Control-Allow-Origin", "*") ->
  setResponseHeader("Access-Control-Allow-Methods", "GET, OPTIONS") ->
  setResponseHeader("Access-Control-Allow-Headers", "Authorization") ->
  status(200) -> <shunt>
```

### Multiple routes

You can also set multiple routes, but you have to set the names of the
route as defined in eskip:

```
zalando.org/skipper-routes: |
  routename1: Path("/") -> localRatelimit(2, "1h") -> inlineContent("A") -> status(200) -> <shunt>;
  routename2: Path("/foo") -> localRatelimit(5, "1h") -> inlineContent("B") -> status(200) -> <shunt>;
```

Make sure the `;` semicolon is used to terminate the routes, if you
use multiple routes definitions.

**Disclaimer**: This feature works only with having different `Path*`
predicates in ingress, if there are no paths rules defined. For
example this will **not** work:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: skipper-ingress
  annotations:
    kubernetes.io/ingress.class: skipper
    zalando.org/skipper-routes: |
       redirect1: Path("/foo/") -> redirectTo(308, "/bar/") -> <shunt>;
  spec:
    rules:
      - host: foo.bar
        http:
          paths:
            - path: /something/
              backend:
                serviceName: something
                servicePort: 80
            - path: /else/
              backend:
                serviceName: else
                servicePort: 80
```

A possible solution will be a skipper route CRD: https://github.com/zalando/skipper/issues/660

## Filters - Basic HTTP manipulations

HTTP manipulations are done by using skipper filters. Changes can be
done in the request path, meaning request to your backend or in the
response path to the client, which made the request.

The following examples can be used within `zalando.org/skipper-filter`
annotation.

### Add a request Header

Add a HTTP header in the request path to your backend.

    setRequestHeader("X-Foo", "bar")

### Add a response Header

Add a HTTP header in the response path of your clients.

    setResponseHeader("X-Foo", "bar")

### Enable gzip

Compress responses with gzip.

    compress() // compress all valid MIME types
    compress("text/html") // only compress HTML files
    compress(9, "text/html") // control the level of compression, 1 = fastest, 9 = best compression, 0 = no compression

### Set the Path

Change the path in the request path to your backend to `/newPath/`.

    setPath("/newPath/")

### Modify Path

Modify the path in the request path from `/api/foo` to your backend to `/foo`.

    modPath("^/api/", "/")

### Set the Querystring

Set the Querystring in the request path to your backend to `?text=godoc%20skipper`.

    setQuery("text", "godoc skipper")

### Redirect

Create a redirect with HTTP code 301 to https://foo.example.org/.

    redirectTo(301, "https://foo.example.org/")

### Cookies

Set a Cookie in the request path to your backend.

    requestCookie("test-session", "abc")

Set a Cookie in the response path of your clients.

    responseCookie("test-session", "abc", 31536000)
    responseCookie("test-session", "abc", 31536000, "change-only")

    // response cookie without HttpOnly:
    jsCookie("test-session-info", "abc-debug", 31536000, "change-only")

### Authorization

Our [autnetication and authorization tutorial](../../tutorials/auth/)
or [filter auth godoc](https://godoc.org/github.com/zalando/skipper/filters/auth)
shows how to use filters for authorization.

#### Basic Auth

    % htpasswd -nbm myName myPassword

    basicAuth("/path/to/htpasswd")
    basicAuth("/path/to/htpasswd", "My Website")

#### Bearer Token (OAuth/JWT)

OAuth2/JWT tokens can be validated and allowed based on different
content of the token. Please check the filter documentation for that:

- [oauthTokeninfoAnyScope](../reference/filters.md#oauthtokeninfoanyscope)
- [oauthTokeninfoAllScope](../reference/filters.md#oauthtokeninfoallscope)
- [oauthTokeninfoAnyKV](../reference/filters.md#oauthtokeninfoanykv)
- [oauthTokeninfoAllKV](../reference/filters.md#oauthtokeninfoallkv)

There are also [auth predicates](../reference/predicates.md#auth), which will allow
you to match a route based on the content of a token:

- `JWTPayloadAnyKV()`
- `JWTPayloadAllKV()`

These are not validating the tokens, which should be done separately
by the filters mentioned above.

### Diagnosis - Throttling Bandwidth - Latency

For diagnosis purpose there are filters that enable you to throttle
the bandwidth or add latency. For the full list of filters see our
[diag filter godoc page](https://godoc.org/github.com/zalando/skipper/filters/diag).

    bandwidth(30) // incoming in kb/s
    backendBandwidth(30) // outgoing in kb/s
    backendLatency(120) // in ms

Filter documentation:

- [latency](../reference/filters.md#latency)
- [bandwidth](../reference/filters.md#bandwidth)
- [chunks](../reference/filters.md#chunks)
- [backendlatency](../reference/filters.md#backendlatency)
- [backendChunks](../reference/filters.md#backendChunks)
- [randomcontent](../reference/filters.md#randomcontent)

### Flow Id to trace request flows

To trace request flows skipper can generate a unique Flow Id for every
HTTP request that it receives. You can then find the trace of the
request in all your access logs.  Skipper sets the X-Flow-Id header to
a unique value. Read more about this in our
[flowid filter](../reference/filters.md#flowid)
and [godoc](https://godoc.org/github.com/zalando/skipper/filters/flowid).

     flowId("reuse")

## Filters - reliability features

Filters can modify http requests and responses. There are plenty of
things you can do with them.

### Circuitbreaker

#### Consecutive Breaker

The [consecutiveBreaker](../reference/filters.md#consecutivebreaker)
filter is a breaker for the ingress route that open if the backend failures
for the route reach a value of N (in this example N=15), where N is a
mandatory argument of the filter and there are some more optional arguments
documented.

    consecutiveBreaker(15)

The ingress spec would look like this:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: consecutiveBreaker(15)
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

#### Rate Breaker

The [rateBreaker](../reference/filters.md#ratebreaker)
filter is a breaker for the ingress route that open if the backend
failures for the route reach a value of N within a window of the last
M requests, where N (in this example 30) and M (in this example 300)
are mandatory arguments of the filter and there are some more optional arguments
documented.

    rateBreaker(30, 300)

The ingress spec would look like this:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: rateBreaker(30, 300)
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```


### Ratelimits

There are two kind of ratelimits:

1. Client side ratelimits are used to slow down login enumeration
attacks, that targets your login pages. This is a security protection
for DDoS or login attacks.
2. Service or backend side ratelimits are used to protect your
services due too much traffic. This can be used in an emergency
situation to make sure you calm down ingress traffic or in general if
you know how much calls per duration your backend is able to handle.
3. Cluster ratelimits can be enforced either on client or on service
side as described above.

Ratelimits are enforced per route.

More details you will find in [ratelimit
package](https://godoc.org/github.com/zalando/skipper/filters/ratelimit)
and in our [ratelimit tutorial](../tutorials/ratelimit.md).

#### Client Ratelimits

The example shows 20 calls per hour per client, based on
X-Forwarded-For header or IP incase there is no X-Forwarded-For header
set, are allowed to each skipper instance for the given ingress.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: localRatelimit(20, "1h")
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

If you need to rate limit service to service communication and
you use Authorization headers to protect your backend from your
clients, then you can pass a 3 parameter to group clients by "Authorization
Header":

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: localRatelimit(20, "1h", "auth")
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```


#### Service Ratelimits

The example shows 50 calls per minute are allowed to each skipper
instance for the given ingress.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: ratelimit(50, "1m")
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

#### Cluster Ratelimits

Cluster ratelimits are eventual consistent and require the flag
`-enable-swarm` to be set.

##### Service

The example shows 50 calls per minute are allowed to pass this ingress
rule to the backend.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: clusterRatelimit("groupSvcApp", 50, "1m")
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

##### Client

The example shows 10 calls per hour are allowed per client,
X-Forwarded-For header, to pass this ingress rule to the backend.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: clusterClientRatelimit("groupSvcApp", 10, "1h")
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

## Shadow Traffic

If you want to test a new replacement of a production service with
production load, you can copy incoming requests to your new endpoint
and ignore the responses from your new backend. This can be done by
the [tee()](../reference/filters.md#tee) and [teenf()](../reference/filters.md#teenf) filters.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: teenf("https://app-new.example.org")
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

## Predicates

[Predicates](../reference/predicates.md)
are influencing the route matching, which you might want to carefully
test before using it in production. This enables you to do feature
toggles or time based enabling endpoints.

You can use all kinds of [predicates](../reference/predicates.md)
with [filters](../reference/filters.md) together.

### Feature Toggle

Feature toggles are often implemented as query string to select a new
feature. Normally you would have to implement this in your
application, but Skipper can help you with that and you can select
routes with an ingress definition.

You create 2 ingresses that matches the same route, here host header
match to `app-default.example.org` and one ingress has a defined query
parameter to select the route to the alpha version deployment. If the
query string in the URL has `version=alpha` set, for example
`https://app-default.example.org/mypath?version=alpha`, the service
`alpha-svc` will get the traffic, if not `prod-svc`.

alpha-svc:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-predicate: QueryParam("version", "^alpha$")
  name: alpha-app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: alpha-svc
          servicePort: 80
```

prod-svc:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: prod-app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: prod-svc
          servicePort: 80
```

### IP Whitelisting

This ingress route will only allow traffic from networks 1.2.3.0/24 and 195.168.0.0/17

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-predicate: Source("1.2.3.0/24", "195.168.0.0/17")
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```


## A/B test

Implementing A/B testing is heavy. Skipper can help you to do
that. You need to have a traffic split somewhere and have your
customers sticky to either A or B flavor of your application. Most
likely people would implement using cookies. Skipper can set a
[cookie with responseCookie()](../reference/filters.md#responsecookie)
in a response to the client and the
[cookie predicate](../reference/predicates.md#cookie)
can be used to match the route based on the cookie. Like this you can
have sticky sessions to either A or B for your clients.  This example
shows to have 10% traffic using A and the rest using B.

10% choice of setting the Cookie "flavor" to "A":

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-predicate: Traffic(.1, "flavor", "A")
    zalando.org/skipper-filter: responseCookie("flavor", "A", 31536000)
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: a-app-svc
          servicePort: 80
```

Rest is setting Cookie "flavor" to "B":

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-filter: responseCookie("flavor, "B", 31536000)
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: b-app-svc
          servicePort: 80
```

To be sticky, you have to create 2 ingress with predicate to match
routes with the cookie we set before. For "A" this would be:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-predicate: Cookie("flavor", /^A$/)
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: a-app-svc
          servicePort: 80
```

For "B" this would be:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-predicate: Cookie("flavor", /^B$/)
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: b-app-svc
          servicePort: 80
```

## Blue-Green deployments

To do blue-green deployments you have to have control over traffic
switching. Skipper gives you the opportunity to set weights to backend
services in your ingress specification. `zalando.org/backend-weights`
is a hash map, which key relates to the `serviceName` of the backend
and the value is the weight of traffic you want to send to the
particular backend. It works for more than 2 backends, but for
simplicity this example shows 2 backends, which should be the default
case for supporting blue-green deployments.

In the following example **my-app-1** service will get **80%** of the traffic
and **my-app-2** will get **20%** of the traffic:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: my-app
  labels:
    application: my-app
  annotations:
    zalando.org/backend-weights: |
      {"my-app-1": 80, "my-app-2": 20}
spec:
  rules:
  - host: my-app.example.org
    http:
      paths:
      - backend:
          serviceName: my-app-1
          servicePort: http
        path: /
      - backend:
          serviceName: my-app-2
          servicePort: http
        path: /
```

For more advanced blue-green deployments, check out our [stackset-controller](https://github.com/zalando-incubator/stackset-controller).

## Chaining Filters and Predicates

You can set multiple filters in a chain similar to the [eskip format](https://godoc.org/github.com/zalando/skipper/eskip).

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    zalando.org/skipper-predicate: Cookie("flavor", /^B$/) && Source("1.2.3.0/24", "195.168.0.0/17")
    zalando.org/skipper-filter: localRatelimit(50, "10m") -> requestCookie("test-session", "abc")
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

## Controlling HTTPS redirect

Skipper Ingress can provide HTTP->HTTPS redirection. Enabling it and setting the status code used by default can
be done with the command line options: `-kubernetes-https-redirect` and `-kubernetes-https-redirect-code`. By using
annotations, this behavior can be overridden from the individual ingress specs for the scope of routes generated
based on these ingresses specs.

Annotations:

- `zalando.org/skipper-ingress-redirect`: the possible values are true or false. When the global HTTPS redirect is
  disabled, the value true enables it for the current ingress. When the global redirect is enabled, the value
  false disables it for the current ingress.
- `zalando.org/skipper-ingress-redirect-code`: the possible values are integers `300 <= x < 400`. Sets the redirect
  status code for the current ingress.

Example:

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-ingress-redirect: "true"
        zalando.org/skipper-ingress-redirect-code: 301
      name: app
    spec:
      rules:
      - host: mobile-api.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

## Loadbalancer Algorithm

You can set the loadbalancer algorithm, which is used to find the next
endpoint for a given request with the ingress annotation
`zalando.org/skipper-loadbalancer`.

For example, for some workloads you might want to have always the same
endpoint for the same client. For this use case there is the
consistent hash algorithm, that finds for a client detected by the IP
or X-Forwarded-For header, the same backend. If the backend is not
available it would switch to another one.

Annotations:

- `zalando.org/skipper-loadbalancer` [see available choices](../../reference/backends/#load-balancer-backend)

Example:

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-loadbalancer: consistentHash
      name: app
    spec:
      rules:
      - host: websocket.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80
