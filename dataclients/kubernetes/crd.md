# RouteGroup CRD

This document is a temporary design document, continously updated as this development branch evolves.

## Goal of the RouteGroup CRD

Provide a format for routing input that makes it easier to benefit more from Skipper's routing features, without
workarounds, than the generic Ingress specs. Primarily targeting the Kubernetes Ingress scenario.

**Goals:**

- more DRY object than ingress (hosts and backends separated from path rules)
- avoid defining unrelated route groups in the same specification object
- better support for skipper specific features
- orchestrate traffic switching for a full set of routes (RouteGroup) without redundant configuration

## Examples

### Minimal example

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - foo.example.org
  destinations:
  - serviceName: foo-service
    servicePort: 80
  paths:
  - path: /
```

### Route + redirect route

It is often required to define a route and a redirect route, which is currently best done with two ingress
objects.

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  destinations:
  - serviceName: foo-service
    servicePort: 80
  paths:
  - path: /login
    method: get
    config:
    - filters:
      - redirectTo(301, "https://login.example.org")
  - path: /
```

### Complex routes

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - www.complex.example.org
  - complex.example.org
  destinations:
  - serviceName: foo-service-v1
    servicePort: 80
  paths:
  - path: /api/resource
    method: post
    config:
    - filters:
      - ratelimit(20, "1m")
      - oauthTokeninfoAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
      predicates:
      - JWTPayloadAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
  - path: /api/resource/*
    method: get
    config:
    - filters:
      - clientRatelimit(100, "1m", "Authorization")
      - oauthTokeninfoAllScope("read.resource", "list.resource")
```

### Traffic switching

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
  annotations:
    zalando.org/backend-weights:
      {"foo-service-v1": 80, "foo-service-v2": 20}
spec:
  destinations:
  - serviceName: foo-service-v1
    servicePort: 80
  - serviceName: foo-service-v2
    servicePort: 80
  paths:
  - path: /api/resource
    method: post
    config:
    - filters:
      - ratelimit(20, "1m")
      - oauthTokeninfoAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
      predicates:
      - JWTPayloadAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
  - path: /api/resource/*
    method:  get
```

### A/B test

A/B test via cookie `canary`, used for sticky sessions.

- 10% chance to get cookie for service-a
- the rest of the traffic goes to service-b

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  destinations:
  - name: variant-a
    serviceName: service-a
    servicePort: 80
  - name: variant-b
    serviceName: service-b
    servicePort: 80
    default: true
  paths:
  - path: /
    config:
    - filters:
      - responseCookie("canary", "A")
      predicates:
      - Traffic(.1)
    destination: variant-a
  - path: /
    config:
    - filters:
      - responseCookie("canary", "B")
  - path: /
    config:
    - predicates:
      - Cookie("canary", "team-foo")
  - path: /
    config:
    - predicates:
      - Cookie("canary", "A")
    destination: variant-a
  - path: /
    config:
    - predicates:
      - Cookie("canary", "B")
```
