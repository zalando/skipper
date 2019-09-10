# RouteGroup Proposal

Date: 2019-09-04
Status: open
Owners: @aryszka @szuecs

This proposal will be kept up to date, if you like to discuss it
please use https://github.com/zalando/skipper/issues/660

Disclaimer:
Skipper's `kubernetes` dataclient will also support Ingress in the
future.

## Purpose

The purpose of the document is to define a RouteGroup CRD, that allows
configuring Skipper for Kubernetes such that it benefits from all the
Skipper features in a straightforward way.

## Problem statement

At Zalando we have a controller, that creates multiple Ingresses for a
given enhanced OpenAPI spec. It configures Skipper features with
[predicates](https://opensource.zalando.com/skipper/reference/predicates)
and
[filters](https://opensource.zalando.com/skipper/reference/filters/)
to allow different rate limits and permissions for different
clients. This works great, but has weaknesses in case of traffic
switching. Right now, traffic switching is orchestrated by
`zalando.org/backend-weights` Ingress annotation.

For example an Ingress object specifies traffic weights for service
`my-app-1` with 80% of the traffic, rest will be routed to
`my-app-2`:

```yaml
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

The problem with having multiple Ingress having this kind of
annotation is that it needs to be in-sync across all Ingress belonging
to a group of routes. This has of course a race condition, which is a
problem in this case.

We want to solve also traffic switching while having ongoing A/B tests
or as simple as defining one or more redirects without having more
than one object.  While you can use `zalando.org/skipper-routes` to do
the redirects, we do not want to move complexity to our users.

## Background Information

A RouteGroup is used to store dependent group of routes in one Object,
that can act as one thing.
Users of this CRD are multiple controllers and end-users.

### Users

#### Controllers

For example [stackset-controller](https://github.com/zalando-incubator/stackset-controller)
orchestrates traffic switching. In this case a set of routes with different
configurations to one backend should be updated atomically.

Another example is an openapi-spec controller, which goal is to create
a set of routes based on API endpoint definitions with rules, defaults
and overrides.

#### End User

An end-user might want to create an Ingress with one or more
additional paths related to the Ingress, that would do redirects.

Another scenario is to refactor parts of an API into a new service.
You want to be able to traffic switch this case, because it is common
enough to be a considered normal case.

### Goals of the RouteGroup CRD

- more DRY object to use (hosts and backends separated from path rules)
- support common use cases in a safe way (redirects, split API)
- avoid defining unrelated route groups in the same specification object
- better support for skipper specific features ([Predicates](https://opensource.zalando.com/skipper/reference/predicates) and [Filters](https://opensource.zalando.com/skipper/reference/filters/))
- orchestrate traffic switching for a full set of routes (RouteGroup) without redundant configuration
- enable complex cases: for example an enhanced openapi-spec controller write this object, instead of creating multiple ingress and [stackset-controller](https://github.com/zalando-incubator/stackset-controller) does traffic switching

## Proposed solution

We create a CRD with `kind: RouteGroup`, that can express the mentioned use cases.
The spec of the RouteGroup has 4 keys that are all arrays of some kind.

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
spec:
  hosts:                    optional
  - <string>
  backends:
  - <backend>
  defaultBackends:          optional
  - <backendRef>
  routes:
  - <route>
```

The `<backend>` object defines, the type of backend with all relevant
information required to create the skipper
[backend](https://opensource.zalando.com/skipper/reference/backends/):

```yaml
<backend>
  name: <string>
  type: <string>            that is one of "service|shunt|loopback|dynamic|lb|network"
  address: <string>         optional, required for type=network
  algorithm: <string>       optional, valid for type=lb
  endpoints: <stringarray>  optional, required for type=lb
  serviceName: <string>     optional, required for type=service Kubernetes
  servicePort: <number>     optional, required for type=service Kubernetes
```

The `defaultBackends` key is a list of `<backendRef>` to be used for
all routes that have no overrides defined. In normal cases the list is
length of 1. The list with more entries is used in case of traffic
switching.

The `<backendRef>` object references the backend by name and adds an
optional weight. The weight is used to split the traffic according to
the definition. If there is no weight defined it will spread traffic
evenly to all backends. To remove traffic from a backend use `weight:0`
or remove the `<backedRef>`

```yaml
<backendRef>
- backendName: <string>
  weight: <number>          optional
```

The `<route>` object defines the actual routing setup and enables skipper specific
[route matching](https://opensource.zalando.com/skipper/tutorials/basics/#route-matching).
In one `<route>` can use
[path](https://opensource.zalando.com/skipper/reference/predicates/#path)
or
[pathSubtree](https://opensource.zalando.com/skipper/reference/predicates/#pathsubtree)
and additionally add a

```yaml
<route>
  path: <string>            either path or pathSubtree is allowed
  pathSubtree: <string>     either path or pathSubtree is allowed
  pathRegexp: <string>      optional
  backends:                 optional, overrides defaults
  - <backendRef>
  filters: <stringarray>    optional
  predicates: <stringarray> optional
  method: <stringarray>     optional, one of the HTTP methods "GET|HEAD|PATCH|POST|PUT|DELETE|CONNECT|OPTIONS|TRACE", defaults to all
```

`<string>` is an arbitrary string
`<stringarray>` is an array of `<string>`
`<number>` is an unsigned integer, example: 80


### RouteGroup expressed use cases

#### Simplicity

To route the listed Host headers `myapp.example.org` and `example.org` to
Kubernetes service `myapp-svc` on service port `80`, the RouteGroup
would look like this:

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: myapp
spec:
  hosts:
  - example.org
  - myapp.example.org
  backends:
  - name: myapp
    type: service
    serviceName: myapp-svc
    servicePort: 80
  routes:
  - path: /
    backends:
    - backendName: myapp
```

Another example, if you have multiple path routes to the same backend
can be done like that:

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: myapp
spec:
  hosts:
  - example.org
  - myapp.example.org
  backends:
  - name: myapp
    type: service
    serviceName: myapp-svc
    servicePort: 80
  defaultBackends:
  - backendName: myapp
  routes:
  - path: /articles
  - path: /articles/shoes
  - path: /order
```

#### Complex route with redirect and migration

Assume we have a Kubernetes service `my-service-v1` which gets all
requests for hostnames www.complex.example.org and
complex.example.org.

We might want to specify a redirect to our login service.

Maybe some day we want to migrate from /api to /, in our backend. Old services should be able to use /api, which they can later change to /.

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - www.complex.example.org
  - complex.example.org
  backends:
  - name: my-service
    type: service
    serviceName: my-service-v1
    servicePort: 80
  - name: redirect
    type: shunt
  - name: migration
    type: loopback
  defaultBackends:
  - backendName: my-service
  routes:
  - path: /
  - path: /api
    backends:
    - backendName: migration
    filters:
    - modPath("/api", "/")
  - path: /login
    method:
    - GET
    backend: redirect
    filters:
    - redirectTo(308, "https://login.example.org/)
```

#### Complex routes with authnz, separating read and write tokens

One use case is to separate read tokens from write tokens for
authorization.


```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: myapp
spec:
  hosts:
  - www.complex.example.org
  - complex.example.org
  backends:
  - name: myapp
    type: service
    serviceName: my-service-v1
    servicePort: 80
  defaultBackends:
  - backendName: my-service
  routes:
  - path: /
    method:
    - PUSH
    - PUT
    - PATCH
    - DELETE
    filters:
    - oauthTokeninfoAllScope("myapp.write")
  - path: /
    method:
    - GET
    - HEAD
    filters:
    - oauthTokeninfoAllScope("myapp.read")
```

#### Complex routes with ratelimits based on tokens

A complex route case could specify different ratelimits for POST and PUT
requests to /api/resource for clients.  Clients that have JWT/OAuth2
Tokens from issuer https://accounts.google.com with email
"important@example.org" get 20/equest per minute, other clients with
Token with issuer https://accounts.google.com or
https://accounts.github.com get only 2.

Additionally for all other HTTP methods ratelimit for each client 100
requests per minute based on "Authorization" header and make sure the
Token is valid and has the scopes to read and list the resource.

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: api
spec:
  hosts:
  - api.example.org
  - api.service.example.net
  backends:
  - name: api-svc
    type: service
    serviceName: api-service-v1
    servicePort: 80
  defaultBackends:
  - backendName: api-svc
  routes:
  - path: /api/resource
    method:
    - post
    - put
    filters:
    - ratelimit(20, "1m")
    - oauthTokeninfoAllKV("iss", "https://accounts.google.com", "email", "important@example.org")
    predicates:
    - JWTPayloadAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
  - path: /api/resource
    method:
    - post
    - put
    filters:
    - ratelimit(2, "1m")
    - oauthTokeninfoAnyKV("iss", "https://accounts.google.com", "iss", "https://accounts.github.com")
  - path: /api/resource
    filters:
    - clientRatelimit(100, "1m", "Authorization")
    - oauthTokeninfoAllScope("read.resource", "list.resource")
```

#### Traffic switching

If we want to traffic switch, we want to make sure all path endpoints
we have configured are switched at the same time and get the same
traffic split applied. To not be ambiguous with multiple backends, we
need to set `default: true` for both backends we traffic switched.

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - api.example.org
  - api.service.example.net
  backends:
  - name: api-svc
    type: service
    serviceName: api-service-v1
    servicePort: 80
    default: true
    weight: 80
  - name: api-svc-v2
    type: service
    serviceName: foo-service-v2
    servicePort: 80
    default: true
    weight: 20
  routes:
  - path: /api/resource
    filters:
    - ratelimit(20, "1m")
    - oauthTokeninfoAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
    predicates:
    - JWTPayloadAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
  - path: /api/resource
```

#### A/B test

A/B test via cookie `canary`, used for sticky sessions.

- 10% chance to get cookie for service-a
- the rest of the traffic goes to service-b

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  backends:
  - name: variant-a
    type: service
    serviceName: service-a
    servicePort: 80
  - name: variant-b
    type: service
    serviceName: service-b
    servicePort: 80
    default: true
  routes:
  - path: /
    filters:
    - responseCookie("canary", "A")
    predicates:
    - Traffic(.1)
    backend: variant-a
  - path: /
    filters:
    - responseCookie("canary", "B")
  - path: /
    predicates:
    - Cookie("canary", "team-foo")
  - path: /
    predicates:
    - Cookie("canary", "A")
    backend: variant-a
  - path: /
    predicates:
    - Cookie("canary", "B")
```

#### A/B test with traffic switching

A/B test via cookie `canary`, used for sticky sessions.

step0
- all traffic goes to Kubernetes service-b-v1

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - api.example.org
  backends:
  - name: variant-b
    type: service
    serviceName: service-b-v1
    servicePort: 80
    default: true
  routes:
  - path: /
```

step1
- A canary route for team "foo" was added, that needs a Cookie "canary" with content "team-foo" to validate service-a

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - api.example.org
  backends:
  - name: variant-a
    type: service
    serviceName: service-a
    servicePort: 80
  - name: variant-b
    type: service
    serviceName: service-b-v1
    servicePort: 80
    default: true
  routes:
  - path: /
  - path: /
    predicates:
    - Cookie("canary", "team-foo")
    backend: variant-a
```

step2
- A/B test: 10% chance to get cookie for service-a
- the rest of the traffic goes to service-b

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - api.example.org
  backends:
  - name: variant-a
    type: service
    serviceName: service-a
    servicePort: 80
  - name: variant-b
    type: service
    serviceName: service-b-v1
    servicePort: 80
    default: true
  routes:
  - path: /
    filters:
    - responseCookie("canary", "A")
    predicates:
    - Traffic(.1)
    backend: variant-a
  - path: /
    filters:
    - responseCookie("canary", "B")
  - path: /
    predicates:
    - Cookie("canary", "team-foo")
  - path: /
    predicates:
    - Cookie("canary", "A")
    backend: variant-a
  - path: /
    predicates:
    - Cookie("canary", "B")
```


step3
- service-b will be traffic switched from v1 to v2, v2 will get 20% traffic, rest to v1

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - api.example.org
  backends:
  - name: variant-a
    type: service
    serviceName: service-a
    servicePort: 80
  - name: variant-b
    type: service
    serviceName: service-b-v1
    servicePort: 80
    default: true
    weight: 80
  - name: variant-b
    type: service
    serviceName: service-b-v2
    servicePort: 80
    default: true
    weight: 20
  routes:
  - path: /
    filters:
    - responseCookie("canary", "A")
    predicates:
    - Traffic(.1)
    backend: variant-a
  - path: /
    filters:
    - responseCookie("canary", "B")
  - path: /
    predicates:
    - Cookie("canary", "team-foo")
  - path: /
    predicates:
    - Cookie("canary", "A")
    backend: variant-a
  - path: /
    predicates:
    - Cookie("canary", "B")
```


### Examples Ingress vs. RouteGroup

#### Minimal example

All requests to one hostname should be routed to a Kubernetes service.

Ingress

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: app
spec:
  rules:
  - host: example.org
    http:
      paths:
      - backend:
          serviceName: app
          servicePort: 80

RouteGroup

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: app
spec:
  hosts:
  - example.org
  backends:
  - name: app
    type: service
    serviceName: app-svc
    servicePort: 80
```

#### 2 Hostnames example

2 different Hostnames should be routed to the same Kubernetes service

Ingress

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: app
spec:
  rules:
  - host: example.org
    http:
      paths:
      - backend:
          serviceName: app
          servicePort: 80
  - host: www.example.org
    http:
      paths:
      - backend:
          serviceName: app
          servicePort: 80

RouteGroup

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: app
spec:
  hosts:
  - example.org
  - www.example.org
  backends:
  - name: app
    type: service
    serviceName: app-svc
    servicePort: 80
```

#### Common use case ingress and redirect

All requests to one hostname with /login in the path should be
redirected to https://login.example.org/, rest of the requests to the
specified hostname should be routed to a Kubernetes service.

Ingress

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: app
  annotations:
    zalando.org/skipper-routes: |
       redirect_app_default: PathRegexp("/login) -> redirectTo(308, "https://login.example.org/") -> <shunt>;
spec:
  rules:
  - host: example.org
    http:
      paths:
      - backend:
          serviceName: app
          servicePort: 80
```

RouteGroup

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: app
spec:
  backends:
  - name: app
    type: service
    serviceName: app-svc
    servicePort: 80
    default: true
  - name: redirect
    type: shunt
  routes:
  - path: /login
    filters:
    - redirectTo(308, "https://login.example.org")
    backend: redirect
  - path: /
```

#### Ingress example for unrelated routes

Ingress

```yaml
spec:
  rules:
  - host: registry.example.org
    http:
      paths:
      - backend:
          serviceName: registry
          servicePort: 80
  - host: shop.foo.com
    http:
      paths:
      path: /api
      - backend:
          serviceName: foo
          servicePort: 80
```

Not possible with a single RouteGroup, because backends can not bind to a host.
