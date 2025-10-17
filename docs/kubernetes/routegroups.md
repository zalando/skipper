# Route groups

Route groups are an alternative to the Kubernetes Ingress format for defining ingress rules. They allow to
define Skipper routing in Kubernetes, while providing a straightforward way to configure the routing features
supported by Skipper and not defined by the generic Ingress.

## Skipper as Kubernetes Ingress controller

Skipper is an extensible HTTP router with rich route matching, and request flow and traffic shaping
capabilities. Through its integration with Kubernetes, it can be used in the role of an ingress controller for
forwarding incoming external requests to the right services in a cluster. Kubernetes provides the Ingress
specification to define the rules by which an ingress controller should handle the incoming traffic. The
specification is simple and generic, but doesn't offer a straightforward way to benefit from Skipper's rich HTTP
related functionality.

## RouteGroups

A RouteGroup is a custom Kubernetes resource definition. It provides a way to define the ingress routing for
Kubernetes services. It allows route matching based on any HTTP request attributes, and provides a clean way for
the request flow augmentation and traffic shaping. It supports higher level features like gradual traffic
switching, A/B testing, and more.

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

(See a more detailed explanation of the above example [further down](#gradual-traffic-switching) in this
document.)

Links:

- [RouteGroup semantics](routegroup-crd.md)
- [CRD definition](https://github.com/zalando/skipper/blob/master/dataclients/kubernetes/deploy/apply/routegroups_crd.yaml)

## Requirements

- [External DNS v0.7.0 or higher](https://github.com/kubernetes-sigs/external-dns/releases/tag/v0.7.0)
- [Kubernetes Ingress Controller for AWS v0.10.0 or higher](https://github.com/zalando-incubator/kube-ingress-aws-controller/releases/tag/v0.10.0)

## Installation

The definition file of the CRD can be found as part of Skipper's source code, at:

https://github.com/zalando/skipper/blob/master/dataclients/kubernetes/deploy/apply/routegroups_crd.yaml

To install it manually in a cluster, assuming the current directory is the root of Skipper's source, call this
command:

```
kubectl apply -f dataclients/kubernetes/deploy/apply/routegroups_crd.yaml
```

This will install a namespaced resource definition, providing the RouteGroup kind:

- full name: routegroups.zalando.org
- resource group: zalando.org/v1
- resource names: routegroup, routegroups, rg, rgs
- kind: RouteGroup

The route groups, once any is defined, can be displayed then via kubectl as:

```
kubectl get rgs
```

The API URL of the routegroup resources will be:

https://kubernetes-api-hostname/apis/zalando.org/v1/routegroups

## Usage

The absolute minimal route group configuration for a Kubernetes service (my-service) looks as follows:

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
  routes:
    - pathSubtree: /
      backends:
        - backendName: my-backend
```

This is equivalent to the ingress:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
spec:
  defaultBackend:
    service:
      name: my-service
      port:
        number: 80
```

Notice that the route group contains a list of actual backends, and the defined service backend is then
referenced as the default backend. This structure plays a role in supporting scenarios like A/B testing and
gradual traffic switching, [explained below](#gradual-traffic-switching). The backend definition also has a type
field, whose values can be service, lb, network, shunt, loopback or dynamic. More details on that
[below](#backends).

Creating, updating and deleting route groups happens the same way as with ingress objects. E.g, manually
applying a route group definition:

```
kubectl apply -f my-route-group.yaml
```

## Hosts

- *[Format](routegroup-crd.md#routegroup-top-level-object)*

Hosts contain hostnames that are used to match the requests handled by a given route group. They are also used
to update the required DNS entries and load balancer configuration if the cluster is set up that way.

Note that it is also possible to use any Skipper predicate in the routes of a route group, with the Host
predicate included, but the hostnames defined that way will not serve as input for the DNS configuration.

## Backends

- *[Format](routegroup-crd.md#backend_1)*
- *[General backend reference](../reference/backends.md)*

RouteGroups support different backends. The most typical backend type is the 'service', and it works the same
way as in case of ingress definitions.

In a RouteGroup, there can be multiple backends and they are listed on the top level of the route group spec,
and are referenced from the actual routes or as default backends.

### type=service

This backend resolves to a Kubernetes service. It works the same way as in case of Ingress definitions. Skipper
resolves the Services to the available Endpoints belonging to the Service, and generates load balanced routes
using them. (This basically means that under the hood, a `service` backend becomes an `lb` backend.)

### type=lb

This backend provides load balancing between multiple network endpoints. Keep in mind that the service type
backend automatically generates load balanced routes for the service endpoints, so this backend type typically
doesn't need to be used for services.

### type=network

This backend type results in routes that proxy incoming requests to the defined network address, regardless of
the Kubernetes semantics, and allows URLs that point somewhere else, potentially outside of the cluster, too.

### type=shunt, type=loopback, type=dynamic, type=forward

These backend types allow advanced routing setups. Please check the [reference
manual](../reference/backends.md) for more details.

## Default Backends

- *[Format](routegroup-crd.md#backend-reference)*

A default backend is a reference to one of the defined backends. When a route doesn't specify which backend(s)
to use, the ones referenced in the default backends will be used.

In case there are no individual routes at all in the route group, a default set of routes (one or more) will be
generated and will proxy the incoming traffic to the default backends.

The reason, why multiple backends can be referenced as default, is that this makes it easy to execute gradual
traffic switching between different versions, even more than two, of the same application. [See
more](#gradual-traffic-switching).

## Routes

- *[Format](routegroup-crd.md#route_1)*

Routes define where to and how the incoming requests will be proxied. The predicates, including the path,
pathSubtree, pathRegexp and methods fields, and any free-form predicate listed under the predicates field,
control which requests are matched by a route, the filters can apply changes to the forwarded requests and the
returned responses, and the backend refs, if defined, override the default backends, where the requests will be
proxied to. If a route group doesn't contain any explicit routes, but it contains default backends, a default
set of routes will be generated for the route group.

Important to bear in mind about the path fields, that the plain 'path' means exact path match, while
'pathSubtree' behaves as a path prefix, and so it is more similar to the path in the Ingress specification.

See also:

- [predicates](../reference/predicates.md)
- [filters](../reference/filters.md)

## Gradual traffic switching

The weighted backend references allow to split the traffic of a single route and send it to different backends
with the ratio defined by the weights of the backend references. E.g:

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-routes
spec:
  hosts:
  - api.example.org
  backends:
  - name: api-svc-v1
    type: service
    serviceName: api-service-v1
    servicePort: 80
  - name: api-svc-v2
    type: service
    serviceName: foo-service-v2
    servicePort: 80
  routes:
  - pathSubtree: /api
    backends:
    - backendName: api-svc-v1
      weight: 80
    - backendName: api-svc-v2
      weight: 20
```

In case of the above example, 80% of the requests is sent to api-service-v1 and the rest is sent to
api-service-v2.

Since this type of weighted traffic switching can be used in combination with the Traffic predicate, it is
possible to control the routing of a long running A/B test, while still executing gradual traffic switching
independently to deploy a new version of the variants, maybe to deploy a fix only to one variant. E.g:

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
  - filters:
    - responseCookie("canary", "A")
    predicates:
    - Traffic(.1)
    backends:
    - backendName: variant-a
  - filters:
    - responseCookie("canary", "B")
  - predicates:
    - Cookie("canary", "A")
    backends:
    - backendName: variant-a
  - predicates:
    - Cookie("canary", "B")
```

See also:

- [Traffic predicate](../reference/predicates.md#traffic)

## Mapping from Ingress to RouteGroups

RouteGroups are one-way compatible with Ingress, meaning that every Ingress specification can be expressed in
the RouteGroup format, as well. In the following, we describe the mapping from Ingress fields to RouteGroup
fields.

### Ingress with default backend

Ingress:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
spec:
  defaultBackend:
    service:
      name: my-service
      port:
        number: 80
```

RouteGroup:

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
  - backendName: my-backend
```

### Ingress with path rule

Ingress:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
spec:
  rules:
  - host: api.example.org
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: my-service
            port:
              number: 80
```

RouteGroup:

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-route-group
spec:
  hosts:
  - api.example.org
  backends:
  - name: my-backend
    type: service
    serviceName: my-service
    servicePort: 80
  routes:
  - pathSubtree: /api
```

### Ingress with multiple hosts

Ingress (we need to define two rules):

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
spec:
  rules:
  - host: api.example.org
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: my-service
            port:
              number: 80
  - host: legacy-name.example.org
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: my-service
            port:
              number: 80
```

RouteGroup (we just define an additional host):

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-route-group
spec:
  hosts:
  - api.example.org
  - legacy-name.example.org
  backends:
  - name: my-backend
    type: service
    serviceName: my-service
    servicePort: 80
  routes:
  - pathSubtree: /api
```

### Ingress with multiple hosts, and different routing

For those cases when using multiple hostnames in the same ingress with different rules, we need to apply a
small workaround for the equivalent route group spec. Ingress:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
spec:
  rules:
  - host: api.example.org
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: my-service
            port:
              number: 80
  - host: legacy-name.example.org
    http:
      paths:
      - path: /application
        pathType: Prefix
        backend:
          service:
            name: my-service
            port:
              number: 80
```

RouteGroup (we need to use additional host predicates):

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-route-group
spec:
  hosts:
  - api.example.org
  - legacy-name.example.org
  backends:
  - name: my-backend
    type: service
    serviceName: my-service
    servicePort: 80
  routes:
  - pathSubtree: /api
    predicates:
    - Host("api.example.org")
  - pathSubtree: /application
    predicates:
    - Host("legacy-name.example.org")
```

The RouteGroups allow multiple hostnames for each route group, but by default, their union is used during
routing. If we want to distinguish between them, then we need to use an additional Host predicate in the routes.
Importantly, only the hostnames listed under the hosts field serve as input for the DNS and LB configuration.

## Mapping Skipper Ingress extensions to RouteGroups

Skipper accepts a set of annotations in Ingress objects that give access to certain Skipper features that would
not be possible with the native fields of the Ingress spec, e.g. improved path handling or rate limiting. These
annotations can be expressed now natively in the RouteGroups.

### zalando.org/backend-weights

Backend weights are now part of the backend references, and they can be controlled for multiple backend sets
within the same route group. See [Gradual traffic switching](#gradual-traffic-switching).

### zalando.org/skipper-filter and zalando.org/skipper-predicate

Filters and predicates are now part of the route objects, and different set of filters or predicates can be set
for different routes.

### zalando.org/skipper-routes

"Custom routes" in a route group are unnecessary, because every route can be configured with predicates, filters
and backends without limitations. E.g where an ingress annotation's metadata may look like this:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
  zalando.org/skipper-routes: |
    Method("OPTIONS") -> status(200) -> <shunt>
spec:
  backend:
    service:
      name: my-service
      port:
        number: 80
```

the equivalent RouteGroup would look like this:

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
  - name: options200
    type: shunt
  defaultBackends:
  - backendName: my-backend
  routes:
  - pathSubtree: /
  - pathSubtree: /
    methods: OPTIONS
    filters:
    - status(200)
    backends:
    - backendName: options200
```

### zalando.org/ratelimit

The ratelimiting can be defined on the route level among the filters, in the same format as in this annotation.

### zalando.org/skipper-ingress-redirect and zalando.org/skipper-ingress-redirect-code

Skipper ingress provides global HTTPS redirect, but it allows individual ingresses to override the global
settings: enabling/disabling it and changing the default redirect code. With route groups, this override can be
achieved by simply defining an additional route, with the same matching rules, and therefore the override can be
controlled eventually on a route basis. E.g:

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
  - name: redirectShunt
    type: shunt
  defaultBackends:
  - backendName: my-backend
  routes:
  - pathSubtree: /
  - pathSubtree: /
    predicates:
    - Header("X-Forwarded-Proto", "http")
    filters:
    - redirectTo(302, "https:")
    backends:
    - backendName: redirectShunt
```

### zalando.org/skipper-loadbalancer

Skipper Ingress doesn't use the ClusterIP of the Service for forwarding the traffic to, but sends it directly to
the Endpoints represented by the Service, and balances the load between them with the round-robin algorithm. The
algorithm choice can be overridden by this annotation. In case of the RouteGroups, the algorithm is simply an
attribute of the backend definition, and it can be set individually for each backend. E.g:

```yaml
  backends:
  - name: my-backend
    type: service
    serviceName: my-service
    servicePort: 80
    algorithm: consistentHash
```

See also:

- [Load Balancer backend](../reference/backends.md#load-balancer-backend)

### zalando.org/skipper-ingress-path-mode

The route objects support the different path lookup modes, by using the path, pathSubtree or the
pathRegexp field. See also the [route matching](../reference/architecture.md#route-matching)
explained for the internals. The mapping is as follows:

Ingress pathType: | RouteGroup:
--- | ---
`Exact` and `/foo`  | path: `/foo`
`Prefix` and `/foo` | pathSubtree: `/foo`

Ingress (`pathType: ImplementationSpecific`): | RouteGroup:
--- | ---
`kubernetes-ingress` and `/foo` | pathRegexp: `^/foo`
`path-regexp` and `/foo` | pathRegexp: `/foo`
`path-prefix` and `/foo` | pathSubtree: `/foo`
`kubernetes-ingress` and /foo$ | path: `/foo`

## Multiple skipper deployments

If you want to split for example `internal` and `public` traffic, it
might be a good choice to split your RouteGroups. Skipper has
the flag `--kubernetes-routegroup-class=<string>` to only select RouteGroup
objects that have the annotation `zalando.org/routegroup.class` set to
`<string>`. Skipper will only create routes for RouteGroup objects with
it's annotation or RouteGroup objects that do not have this annotation. The
default class is `skipper`, if not set.

Example RouteGroup:

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: my-route-group
  annotations:
    zalando.org/routegroup.class: internal
spec:
  backends:
  - name: my-backend
    type: service
    serviceName: my-service
    servicePort: 80
  defaultBackends:
  - backendName: my-service
```
