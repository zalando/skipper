# RouteGroup CRD Semantics

This document contains the semantic definition of the RouteGroup CRD. For more information, see the [route group
documentation](routegroups.md), or see the [CRD yaml
definition](https://github.com/zalando/skipper/blob/master/dataclients/kubernetes/deploy/apply/routegroups_crd.yaml).

## Concepts

### RouteGroup

A RouteGroup represents a grouped routing specification, with one or more backends, typically a Kubernetes
service. The Skipper routes yielded by a route group are handled atomically, meaning that if any problem is
detected during processing a route group, none of the generated routes from that group will be applied.

### Hosts

A list of allowed DNS host names that an incoming HTTP request should match in order to be handled by the route
group. Host list is mandatory.

### Backend

Typically a Kubernetes service, but not necessarily. The routes generated from route groups need to have a
backend, therefore at least one backend is mandatory.

### Default backend

A route group can contain multiple routes. If the routes don't identify the backend, then the default backends
are used. There can be multiple default backends, e.g. to support weighted A/B testing.

### Route

Routes describe how a matching HTTP request is handled and where it is forwarded to.

### Predicate

A predicate is used during route lookup to identify which route should handle an incoming request. Route group
routes provide dedicated fields for the most common predicates like the path or the HTTP method, but in the
predicates list field, it is possible to define and configure any predicate supported by Skipper. See the
[Predicates](../reference/predicates.md) section of the reference.

### Filter

A filter is used during handling the request to shape the request flow. In a route group, any filter supported
by Skipper is allowed to be used. See the [Filters](../reference/filters.md)
section of the reference.

## RouteGroup - top level object

The route group spec must contain hosts, backends, routes and optional default backends.

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
spec:
  hosts:
  - <string>
  backends:
  - <backend>
  defaultBackends:
  - <backendRef>
  routes:
  - <route>
```

## Backend

The `<backend>` object defines the type of a backend and the required configuration based on the type. Required
fields are the name and the type, while the rest of the fields may be required based on the type.

```yaml
<backend>
  name: <string>
  type: <string>            one of "service|shunt|loopback|dynamic|lb|network|forward"
  address: <string>         optional, required for type=network
  algorithm: <string>       optional, valid for type=lb|service, values=roundRobin|random|consistentHash|powerOfRandomNChoices
  endpoints: <stringarray>  optional, required for type=lb
  serviceName: <string>     optional, required for type=service
  servicePort: <number>     optional, required for type=service
```

See more about Skipper backends in the [backend documentation](../reference/backends.md).

## Backend reference

The `<backendRef>` object references a backend that is defined in the route group's backends field. The name is
a required field, while the weight is optional. If no weight is used at all, then the traffic is split evenly
between the referenced backends. One or more backend reference may appear on the route group level as a default
backend, or in a route.

```yaml
<backendRef>
- backendName: <string>
  weight: <number>          optional
```

## Route

The `<route>` object defines the actual routing setup with custom matching rules (predicates), and request flow
shaping with filters.

```yaml
<route>
  path: <string>            either path or pathSubtree is allowed
  pathSubtree: <string>     either path or pathSubtree is allowed
  pathRegexp: <string>      optional
  methods: <stringarray>    optional, one of the HTTP methods per entry "GET|HEAD|PATCH|POST|PUT|DELETE|CONNECT|OPTIONS|TRACE", defaults to all
  predicates: <stringarray> optional
  filters: <stringarray>    optional
  backends:                 optional, overrides defaults
  - <backendRef>
```

The `path`, `pathSubtree` and `pathRegexp` fields work the same way as the predicate counterparts on eskip
routes. See the [reference manual](../reference/predicates.md) for more details.

The `methods` field defines which methods an incoming request can have in order to match the route.

The items in the `predicates` and `filter` fields take lists of predicates and filters, respectively, defined in
their eskip format. Example:

```yaml
  predicates:
  - Cookie("alpha", "enabled")
  - Header("X-Test", "true")
  filters:
  - setQuery("test", "alpha")
  - compress()
```

See also:

- [predicates](../reference/predicates.md)
- [filters](../reference/filters.md)

The <backendRef> references in the backends field, if present, define which backends a route should use.
