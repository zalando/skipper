what does skipper do
what does skipper in kubernetes do
what are route groups
why trying route groups instead of ingress
take stuff from the crd
link the crd
write markdown docs
write godoc
how to install it
current limitations
how to use it
ingress features
route group specific features
differences between route groups and ingress
east-west handling
review the existing kubernetes docs for things that are common to ingress and route groups

# Route groups

Route groups are an alternative to Kubernetes Ingress. They allow to define Skipper routing in Kubernetes, while
providing a straightforward way to configure Skipper's routing features.

The integration with the LB and DNS configuration solutions is work in progress.

## Skipper as Kubernetes Ingress controller

skipper a router
selects routes and applies request/response augmentation
service architecture features like gradual traffic switching between versions, canary testing
config dynamic, various extensible set of sources
ingress one such source

Skipper is an extensible HTTP router with rich route matching, and request flow and traffic shaping
capabilities. Through its integration with Kubernetes, it can be used in the role of an ingress controller for
routing incoming external requests to the right services in a cluster. Kubernetes provides the Ingress
specification to define the rules by which an ingress controller should route the incoming traffic. The
specification is simple and generic, but doesn't offer a straightforward way to benefit from Skipper's rich HTTP
related functionality.

## RouteGroups

an alternative format for defining ingress rules
route selection on all HTTP features and custom ones
straightforward way of expressing request/response flow augmentation
skipper routes yes, but multiple routes for an application or service grouped together and handled atomically
link to crd comparison, we pointed out some limitations problems here

A RouteGroup is a custom Kubernetes resource definition. It provides a way to define the ingress routing for
Kubernetes services. It allows route matching based on any HTTP request attributes, and provides a clean for the
request flow augmentation and traffic shaping. It supports higher level features like gradual traffic switching,
A/B testing, and more.

Example:

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

Links:
- [RouteGroup semantics](https://github.com/zalando/skipper/blob/master/dataclients/kubernetes/routegroup-crd.md)
- [CRD definition](https://github.com/zalando/skipper/blob/master/dataclients/kubernetes/deploy/routegroups/apply/routegroups_crd.yaml)

## Current Limitations

LB
DNS

## Installation

## Usage

The absolute minimal route group configuration for Kubernetes service (my-service) looks as follows:

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
  defaultBackends:
  - backendName: my-service
```

Notice that 

- commands

## Ingress to RouteGroups

## Ingress Skipper extensions to RouteGroups
