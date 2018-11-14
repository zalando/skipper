## East-West Usage

If you run Skipper with an [East-West
setup](../ingress-controller/#run-as-api-gateway-with-east-west-setup),
you can use the configured ingress also to do service-to-service
calls, bypassing your ingress loadbalancer and stay inside the
cluster. It depends on the configuration, but the default is that you
can connect via HTTP to  `<name>.<namespace>.skipper.cluster.local`
to your application based on the ingress configuration.

Example:

```
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: demo
  namespace: default
spec:
  rules:
  - host: demo.example.org
    http:
      paths:
      - backend:
          serviceName: example
          servicePort: 80
```

Your clients inside the cluster should call this example with
`demo.default.skipper.cluster.local` in their host header. Example
from inside a container:

```
curl http://demo.default.skipper.cluster.local/
```

Metrics will change, because skipper stores metrics per HTTP Host
header, which changes with cluster internal calls from
`demo.example.org` to `demo.default.skipper.cluster.local`.

You can use all features as defined in [Ingress
Usage](../ingress-usage/), [Filters](../../reference/filters/),
[Predicates](../../reference/predicates/) via [annotations as
before](../ingress-usage/#filters-and-predicates) and also [custom-routes](../ingress-usage/#custom-routes).
