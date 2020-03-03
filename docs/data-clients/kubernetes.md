# Kubernetes

Skipper's Kubernetes dataclient can be used, if you want to run Skipper as
[kubernetes-ingress-controller](https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-controllers).
It will get its route information from provisioned
[Ingress Objects](https://kubernetes.io/docs/concepts/services-networking/ingress).
Detailed information you find in our [godoc for dataclient kubernetes](https://godoc.org/github.com/zalando/skipper/dataclients/kubernetes).

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
ingress.yaml files to build resiliency patterns like ratelimit or
circuitbreaker. You can also use them to build more highlevel
deployment patterns, for example feature toggles, shadow traffic or
blue-green deployments.

Skipper's main features:

- Filters - create, update, delete all kind of HTTP data
  - [collection of base http manipulations](https://godoc.org/github.com/zalando/skipper/filters/builtin): for example manipulating Path, Querystring, ResponseHeader, RequestHeader and redirect handling
  - [cookie handling](https://godoc.org/github.com/zalando/skipper/filters/cookie)
  - [circuitbreakers](https://godoc.org/github.com/zalando/skipper/filters/circuit): consecutiveBreaker or rateBreaker
  - [ratelimit](https://godoc.org/github.com/zalando/skipper/filters/ratelimit): based on client or backend data
  - Shadow traffic: [tee()](https://godoc.org/github.com/zalando/skipper/filters/tee)
- Predicates - advanced matching capability
  - URL Path match: `Path("/foo")`
  - Host header match: `Host("^www.example.org$")`
  - [Querystring](https://godoc.org/github.com/zalando/skipper/predicates/query): `QueryParam("featureX")`
  - [Cookie based](https://godoc.org/github.com/zalando/skipper/predicates/cookie): `Cookie("alpha", /^enabled$/)`
  - [source whitelist](https://godoc.org/github.com/zalando/skipper/predicates/source): `Source("1.2.3.4/24")`
  - [time based interval](https://godoc.org/github.com/zalando/skipper/predicates/interval)
  - [traffic by percentage](https://godoc.org/github.com/zalando/skipper/predicates/traffic) supports also sticky sessions
- Kubernetes integration
  - All Filters and Predicates can be used with 2 annotations
    - Predicates: `zalando.org/skipper-predicate`
    - Filters: `zalando.org/skipper-filter`
  - Custom routes can be defined with the annotation `zalando.org/skipper-routes`
  - [metrics](https://godoc.org/github.com/zalando/skipper/metrics)
  - access logs
  - Blue-Green deployments, with another Ingress annotation `zalando.org/backend-weights`
