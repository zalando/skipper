# Kubernetes

Skipper's Kubernetes dataclient can be used, if you want to run Skipper as
[kubernetes-ingress-controller](https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-controllers).
It will get its route information from provisioned
[Ingress Objects](https://kubernetes.io/docs/concepts/services-networking/ingress).

## Kubernetes Ingress Controller deployment

How to [install Skipper ingress-controller](../kubernetes/ingress-controller.md) for cluster operators.

## Kubernetes Ingress Usage

Find out more [how to use Skipper ingress features](../kubernetes/ingress-usage.md) for deployers.

## Why to choose Skipper?

Kubernetes is a fast changing environment and traditional http routers
are not made for frequently changing routing tables. Skipper is a http
proxy made to apply updates very often. Skipper is used in
production with more than 200.000 routing table entries.
Skipper has Filters to change http data and Predicates to change the
matching rules, both can combined and chained. You can set these in
ingress.yaml files to build resiliency patterns like load shedding, ratelimit or
circuitbreaker. You can also use them to build more high-level
deployment patterns, for example feature toggles, shadow traffic or
blue-green deployments.

Skipper's main features:

- Filters - create, update, delete all kind of HTTP data
   - [collection of base http manipulations](../reference/filters.md):
     for example [manipulating Path](../reference/filters.md#http-path), [Querystring](../reference/filters.md#http-query), [HTTP Headers](../reference/filters.md#http-headers) and [redirect](../reference/filters.md#http-redirect) handling
   - [cookie handling](../reference/filters.md#cookie-handling)
   - [circuitbreakers](../reference/filters.md#circuit-breakers)
   - [ratelimit](../reference/filters.md#rate-limit): based on client or backend data
   - [Shadow traffic filters](../reference/filters.md#shadow-traffic)
- [Predicates](../reference/predicates.md) - advanced matching capability
   - URL [Path](../reference/predicates.md#the-path-tree) match: `Path("/foo")`
   - [Host header](../reference/predicates.md#host) match: `Host("^www.example.org$")`
   - [Querystring](../reference/predicates.md#queryparam): `QueryParam("featureX")`
   - [Cookie based](../reference/predicates.md#cookie): `Cookie("alpha", /^enabled$/)`
   - [source IP allowlist](../reference/predicates.md#source): `Source("1.2.3.4/24")` or `ClientIP("1.2.3.4/24")`
   - [time based interval](../reference/predicates.md#interval)
   - [traffic by percentage](../reference/predicates.md#trafficsegment) supports also sticky sessions
- Kubernetes integration
   - All Filters and Predicates can be used with 2 [annotations](../kubernetes/ingress-usage.md#skipper-ingress-annotations)
      - Predicates: `zalando.org/skipper-predicate`
      - Filters: `zalando.org/skipper-filter`
   - Custom routes can be defined with the annotation `zalando.org/skipper-routes`
   - [RouteGroup CRD](../kubernetes/routegroups.md) to support all skipper features without limitation
   - [monitoring](../operation/operation.md#monitoring)
   - [opentracing](../operation/operation.md#opentracing)
   - access logs with fine granular control of logs by status codes
   - Blue-Green deployments, with another Ingress annotation `zalando.org/backend-weights`
