## East-West Usage

If you run Skipper with an [East-West
setup](ingress-controller.md#run-as-api-gateway-with-east-west-setup),
you can use the configured ingress also to do service-to-service
calls, bypassing your ingress loadbalancer and stay inside the
cluster. You can connect via HTTP to your application based on its
ingress configuration.

Example:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: demo
  namespace: default
spec:
  rules:
  - host: demo.skipper.cluster.local
    http:
      paths:
      - backend:
          service:
            name: example
            port:
              number: 80
        pathType: ImplementationSpecific
```

Or as a [RouteGroup](./routegroups.md):

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: demo
  namespace: default
spec:
  hosts:
  - demo.skipper.cluster.local
  backends:
  - name: backend
    type: service
    serviceName: example
    servicePort: 80
  defaultBackends:
  - backendName: backend
```

Your clients inside the cluster should call this example with
`demo.skipper.cluster.local` in their host header. Example
from inside a container:

```
curl http://demo.skipper.cluster.local/
```

You can also use the same ingress or RouteGroup object to accept
internal and external traffic:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: demo
  namespace: default
spec:
  rules:
  - host: demo.example.com
    http:
      paths:
      - backend:
          service:
            name: example
            port:
              number: 80
        pathType: ImplementationSpecific
  - host: demo.skipper.cluster.local
    http:
      paths:
      - backend:
          service:
            name: example
            port:
              number: 80
        pathType: ImplementationSpecific
```

Or, again, as a [RouteGroup](./routegroups.md):

```yaml
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: demo
  namespace: default
spec:
  hosts:
  - demo.skipper.cluster.local
  - demo.example.com
  backends:
  - name: backend
    type: service
    serviceName: example
    servicePort: 80
  defaultBackends:
  - backendName: backend
```

Metrics will change, because skipper stores metrics per HTTP Host
header, which changes with cluster internal calls from
`demo.example.org` to `demo.default.skipper.cluster.local`.

You can use all features as defined in [Ingress
Usage](ingress-usage.md), [Filters](../reference/filters.md),
[Predicates](../reference/predicates.md) via [annotations as
before](ingress-usage.md#filters-and-predicates) and also [custom-routes](ingress-usage.md#custom-routes).
