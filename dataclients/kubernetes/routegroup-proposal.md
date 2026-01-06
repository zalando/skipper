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

We want to solve common user problems and observed unsafe usage of
Ingress.

### Redirects

One common case is, that users want to define one or more redirects on
an Ingress. There are two solutions in skipper.

First you can create one Ingress for your application and one for each
redirect. The redirect would be done by matching the request with
annotation `zalando.org/skipper-predicate` and applying the
[redirectTo filter](https://opensource.zalando.com/skipper/reference/filters/#redirectto)
with `zalando.org/skipper-filter` annotation. This solution needs more
maintenance and assigning all Ingresses a valid Kubernetes
service as a backend, that is not used for a redirect.

As second solution you can use `zalando.org/skipper-routes`. It does
not require you to have more than one object. For these routes you
have to specify a unique eskip route ID and you have to apply eskip
syntax as a string, which is not validated in Kubernetes. Like this
you can easily create errors and we want to reduce the unsafe
usage. We should reduce complexity to our users.

### Traffic switching complex routes

Right now the traffic switching orchestration works based on Ingress
annotation `zalando.org/backend-weights`, which supports an arbitrary
number of backends. This supports one Ingress to be traffic switched
automatically using
[stackset-controller](https://github.com/zalando-incubator/stackset-controller)
for example. You can also have 3 versions with an ongoing traffic switch.

This solution lacks of traffic switching with more than one Ingress
object as the same stackset.

We want to be able to also do traffic switching, while having ongoing
A/B tests, that require routing with cookies, for example. This
complex routing needs either more than one object or rely on the less
safe use of `zalando.org/skipper-routes`.

### Advanced routing - Open API Spec controller

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

The problem with having multiple Ingresses with this kind of
annotation is that it needs to be in-sync across all Ingresses belonging
to a group of routes. While the orchestrating controller writes these
objects, skipper instances read these. This can result in unexpected
behavior, caused by non atomic changes. More problematic could be if
an API changes and it requires an atomic change to have either the
version 1 view or the version 2 view and not half.

## Background Information

A RouteGroup is used to store dependent group of routes in one Object,
that can act as one thing.
Users of this CRD are multiple controllers and end-users.

### Users

#### Controllers

For example [stackset-controller](https://github.com/zalando-incubator/stackset-controller)
orchestrates traffic switching. In this case a set of routes with different
configurations to one backend should be updated atomically.

Another example is an openapi-spec controller, whose goal is to create
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
- enable complex cases: for example an enhanced openapi-spec controller writes this object, instead of creating
  multiple ingresses and [stackset-controller](https://github.com/zalando-incubator/stackset-controller) does traffic switching

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
  type: <string>            one of "service|shunt|loopback|dynamic|lb|network"
  address: <string>         optional, required for type=network
  algorithm: <string>       optional, valid for type=lb|service, values=roundRobin|random|consistentHash|powerOfRandomNChoices
  endpoints: <stringarray>  optional, required for type=lb
  serviceName: <string>     optional, required for type=service
  servicePort: <number>     optional, required for type=service
```

The `defaultBackends` key is a list of `<backendRef>` to be used for
all routes that have no overrides defined. In normal cases the list is
of length 1. The list with more entries is used in case of traffic
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
and additionally add an optional
[pathRegexp](https://opensource.zalando.com/skipper/reference/predicates/#pathregexp).

```yaml
<route>
  path: <string>            either path or pathSubtree is allowed
  pathSubtree: <string>     either path or pathSubtree is allowed
  pathRegexp: <string>      optional
  backends:                 optional, overrides defaults
  - <backendRef>
  filters: <stringarray>    optional
  predicates: <stringarray> optional
  methods: <stringarray>    optional, one of the HTTP methods per entry "GET|HEAD|PATCH|POST|PUT|DELETE|CONNECT|OPTIONS|TRACE", defaults to all
```

`<string>` is an arbitrary string
`<stringarray>` is an array of `<string>`
`<number>` is an unsigned integer, example: 80


### RouteGroup expressed use cases

#### Simplicity

To route the listed Host headers `myapp.example.org` and `example.org`
and all paths the client chose to a Kubernetes service `myapp-svc` on
service port `80`, the RouteGroup would look like this:

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
  - pathSubtree: /
    backends:
    - backendName: myapp
```

Another example, if you have multiple exact path routes to the same
backend can be done like that:

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
  - pathSubtree: /
  - pathSubtree: /api
    backends:
    - backendName: migration
    filters:
    - modPath("/api", "/")
  - path: /login
    methods:
    - GET
    backends:
    - backendName: redirect
    filters:
    - redirectTo(308, "https://login.example.org/")
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
  - backendName: myapp
  routes:
  - pathSubtree: /
    methods:
    - PUSH
    - PUT
    - PATCH
    - DELETE
    filters:
    - oauthTokeninfoAllScope("myapp.write")
  - pathSubtree: /
    methods:
    - GET
    - HEAD
    filters:
    - oauthTokeninfoAllScope("myapp.read")
```

#### Complex routes with ratelimits based on tokens

A complex route case could specify different ratelimits for POST and PUT
requests to /api/resource for clients. Clients that have JWT/OAuth2
Tokens from issuer https://accounts.google.com with email
"important@example.org" get 20/equest per minute, other clients with
Token with issuer https://accounts.google.com or
https://accounts.github.com get only 2.

Additionally for all other HTTP methods ratelimit for each client 10
requests per hour based on "Authorization" header and make sure the
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
    methods:
    - post
    - put
    filters:
    - ratelimit(20, "1m")
    - oauthTokeninfoAllKV("iss", "https://accounts.google.com", "email", "important@example.org")
    predicates:
    - JWTPayloadAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
  - path: /api/resource
    methods:
    - post
    - put
    filters:
    - ratelimit(2, "1m")
    - oauthTokeninfoAnyKV("iss", "https://accounts.google.com", "iss", "https://accounts.github.com")
  - path: /api/resource
    filters:
    - clientRatelimit(10, "1h", "Authorization")
    - oauthTokeninfoAllScope("read.resource", "list.resource")
```

#### Traffic switching

If we want to switch traffic, we want to make sure all path endpoints
we have configured are switched at the same time and get the same
traffic split applied. To not be ambiguous with multiple backends, we
need to set the `weight` and both backends have to be
`defaultBackends`.

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
  - name: api-svc-v2
    type: service
    serviceName: foo-service-v2
    servicePort: 80
  defaultBackends:
  - backendName: api-svc
    weight: 80
  - backendName: api-svc-v2
    weight: 20
  routes:
  - path: /api/resource
    filters:
    - ratelimit(200, "1m")
    - oauthTokeninfoAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
    predicates:
    - JWTPayloadAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
  - path: /api/resource
    filters:
    - ratelimit(20, "1m")
    - oauthTokeninfoAllKV("iss", "https://accounts.google.com")
```

#### A/B test

A/B test via cookie `canary`, used for sticky sessions.

- 10% chance to get cookie for service-a
- the rest of the traffic goes to service-b
- All requests with a `canary` cookie will be sticky to the chosen backend

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
  defaultBackends:
  - backendName: variant-b
  routes:
  - pathSubtree: /
    filters:
    - responseCookie("canary", "A") # set canary Cookie to A
    predicates:
    - Traffic(.1)                   # 10% chance
    backends:
    - backendName: variant-a        # overrides default
  - pathSubtree: /
    filters:
    - responseCookie("canary", "B")
  - pathSubtree: /
    predicates:
    - Cookie("canary", "A")         # sticky match
    backends:
    - backendName: variant-a        # overrides default
  - pathSubtree: /
    predicates:
    - Cookie("canary", "B")
```

#### A/B test with traffic switching

A/B test via cookie `canary`, used for sticky sessions.

**step0**
- all traffic goes to backend `variant-b`, Kubernetes service-b-v1

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
  defaultBackends:
  - backendName: variant-b
  routes:
  - pathSubtree: /
```

**step1**
- A canary route for team foo was added, that needs a Cookie `canary`
  with content `team-foo` to validate backend `variant-a`, before
  customers will get the traffic.

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
  defaultBackends:
  - backendName: variant-b
  routes:
  - pathSubtree: /
  - pathSubtree: /
    predicates:
    - Cookie("canary", "team-foo")
    backends:
    - backendName: variant-a
```

**step2**
- After successful test delete team cookie
- A/B test: 10% chance to get cookie for backend `variant-a`
- the rest of the traffic goes to backend `variant-b`

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
  defaultBackends:
  - backendName: variant-b
  routes:
  - pathSubtree: /
    filters:
    - responseCookie("canary", "A")
    predicates:
    - Traffic(.1)
    backends:
    - backendName: variant-a
  - pathSubtree: /
    filters:
    - responseCookie("canary", "B")
  - pathSubtree: /
    predicates:
    - Cookie("canary", "A")
    backends:
    - backendName: variant-a
  - pathSubtree: /
    predicates:
    - Cookie("canary", "B")
```

**step3**
- service-b will be traffic switched from v1 to v2, v2 will get 20% traffic, rest of the request goes to v1
- service-a stays unchanged and will serve all customers that have cookie `canary=A`

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
  - name: variant-b-v2
    type: service
    serviceName: service-b-v2
    servicePort: 80
  defaultBackends:
  - backendName: variant-b
    weight: 80
  - backendName: variant-b-v2
    weight: 20
  routes:
  - pathSubtree: /
    filters:
    - responseCookie("canary", "A")
    predicates:
    - Traffic(.1)
    backends:
    - backendName: variant-a
  - pathSubtree: /
    filters:
    - responseCookie("canary", "B")
  - pathSubtree: /
    predicates:
    - Cookie("canary", "A")
    backends:
    - backendName: variant-a
  - pathSubtree: /
    predicates:
    - Cookie("canary", "B")
```

**step4**
- variant-b will be completely switched to `service-b-v2`
- variant-a stays unchanged and all customers that have cookie `canary=A`

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
    serviceName: service-b-v2
    servicePort: 80
  defaultBackends:
  - backendName: variant-b
  routes:
  - pathSubtree: /
    filters:
    - responseCookie("canary", "A")
    predicates:
    - Traffic(.1)
    backends:
    - backendName: variant-a
  - pathSubtree: /
    filters:
    - responseCookie("canary", "B")
  - pathSubtree: /
    predicates:
    - Cookie("canary", "A")
    backends:
    - backendName: variant-a
  - pathSubtree: /
    predicates:
    - Cookie("canary", "B")
```

As we see in **step0** till **step4** we can create an A/B test with
manual pre-validating `variant-a`. We do ongoing A/B test and can do
traffic switching for one backend while having a running A/B test.

### Examples Ingress vs. RouteGroup

Ingress objects are well known. To compare the use of RouteGroup we
show some Ingress examples and how these would be written as RouteGroup.

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
  defaultBackends:
  - backendName: app
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
  defaultBackends:
  - backendName: app
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
  - name: redirect
    type: shunt
  defaultBackends:
  - backendName: app
  routes:
  - path: /login
    filters:
    - redirectTo(308, "https://login.example.org")
    backends:
    - backendName: redirect
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
      - path: /api
        backend:
          serviceName: foo
          servicePort: 80
```

Not possible with a single RouteGroup, because backends cannot bind to a host.

#### Traffic Switching with 2 hostnames

Traffic switch state
- api-svc-v1: 70%
- api-svc-v2: 30%

Ingress

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: app
  annotations:
    zalando.org/backend-weights: {"api-svc-v1": 70, "api-svc-v2": 30}
spec:
  rules:
  - host: api.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc-v1
          servicePort: 80
  - host: api.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc-v2
          servicePort: 80
  - host: example.org
    http:
      paths:
      - backend:
          serviceName: app-svc-v1
          servicePort: 80
  - host: example.org
    http:
      paths:
      - backend:
          serviceName: app-svc-v2
          servicePort: 80
```

RouteGroup

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - api.example.org
  - example.org
  backends:
  - name: api-svc
    type: service
    serviceName: api-service-v1
    servicePort: 80
  - name: api-svc-v2
    type: service
    serviceName: foo-service-v2
    servicePort: 80
  defaultBackends:
  - backendName: api-svc
    weight: 70
  - backendName: api-svc-v2
    weight: 30
```

### External components that need to be changed

The current Kubernetes loadbalancer infrastructure, that we provide
automates ALB creation, DNS records and do the HTTP routing. HTTP
routing is the only thing that skipper does and the other tools need
also to be changed to make `RouteGroup` a useful tool to the user.

#### Kube-ingress-aws-controller

[Kube-ingress-aws-controller](https://github.com/zalando-incubator/kube-ingress-aws-controller)
needs to be changed to create the load balancer based on the
`RouteGroup`. It also needs to update the status field similar to the
Ingress status field, in order to provide data for other controller,
for example external-dns, to work on that.

Ingress status field, set by kube-ingress-aws-controller:

```json
{
  "loadBalancer": {
    "ingress": [
      {
        "hostname": "kube-ing-lb-52qc7m7pofvpj-1564137991.eu-west-1.elb.amazonaws.com"
      }
    ]
  }
}
```

It would need to read `RouteGroup.spec.hosts` to get all hostnames to
find certificates.
It would need to write into the status field the created ALIAS record
for the ALBs created for the `RouteGroup`.

#### External-DNS

[External-DNS](https://github.com/kubernetes-incubator/external-dns/)
needs to be able to consume the status field and `hosts` spec of the
proposed `RoutingGroup` to create the DNS records.

#### StackSet Controller

[Stackset-controller](https://github.com/zalando-incubator/stackset-controller)
needs to have a `routegroup` spec definition, manage a `RouteGroup`
and write to the `RouteGroup` annotation in order to be able to do
traffic switching based on a `RouteGroup`.

### Why not adapt other solutions?

In general we want to increase usability for more complex Ingress
cases and enable all Skipper features.

Some Ingress controllers use heavily annotations to expose their
capabilities, as skipper does with
[skipper annotations](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/#skipper-ingress-annotations),
for example [Nginx exposes a lot of annotations](https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/annotations/).

The drawback of annotations is, that these are not type safe in
Kubernetes and can only be validated by the controller after
successful apply by the user.
Another problem is, that annotations from the metadata are disturbing
the mental concept of the spec. The spec should define what you want,
but now you have to patch the spec with all the annotations you
applied.

Another solution would be to adapt
[SMI](https://github.com/deislabs/smi-spec). If you check the
[routing specs](https://github.com/deislabs/smi-spec/blob/master/traffic-specs.md),
you do not find the concept of respond from HTTP router, which is one
capability in Skipper, that is used to create redirects. This we would
have to implement again with annotations. Another Skipper capability, which we
would lack is to define the different path predicates: `Path()`,
`PathSubtree()` and `PathRegexp()`. SMI only define routes as
`pathRegex`. To enable the mentioned path predicates, we would need to
have this unrelated name and change the behavior based on annotations,
which is not increased usability.

Another solution would be to adapt [Istio
VirtualService](https://istio.io/docs/reference/config/networking/v1alpha3/virtual-service/),
which seems the most similar object to the proposed
`RouteGroup`. [HTTPMatchRequest](https://istio.io/docs/reference/config/networking/v1alpha3/virtual-service/#HTTPMatchRequest)
would be able to reflect Skipper's path routing capabilities and you
can define
[HTTPRedirect](https://istio.io/docs/reference/config/networking/v1alpha3/virtual-service/#HTTPRedirect).

The problem of VirtualService object is that it does not reflect
Skipper filters and predicates. There are a number of predicates and
filters, that cannot be used from the VirtualService and we would
need to map every single key to a specific predicate or filter. This
increases the complexity of the code base. Skipper also lacks of the
L4 capabilities, such that we would not be 100% compliant to the spec.
In our opinion the VirtualService is also a quite complex type, that
does not increase the usability.
