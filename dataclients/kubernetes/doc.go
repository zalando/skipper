/*
Package kubernetes implements Kubernetes Ingress support for Skipper.

See: http://kubernetes.io/docs/user-guide/ingress/

The package provides a Skipper DataClient implementation that can be used to access the Kubernetes API for
ingress resources and generate routes based on them. The client polls for the ingress settings, and there is no
need for a separate controller. On the other hand, it doesn't provide a full Ingress solution alone, because it
doesn't do any load balancer configuration or DNS updates. For a full Ingress solution, it is possible to use
Skipper together with Kube-ingress-aws-controller, which targets AWS and takes care of the load balancer setup
for Kubernetes Ingress.

See: https://github.com/zalando-incubator/kube-ingress-aws-controller

Both Kube-ingress-aws-controller and Skipper Kubernetes are part of the larger project, Kubernetes On AWS:

https://github.com/zalando-incubator/kubernetes-on-aws/

Ingress shutdown by healthcheck

The Kubernetes ingress client catches TERM signals when the ProvideHealthcheck option is enabled, and reports
failing healthcheck after the signal was received. This means that, when the Ingress client is responsible for
the healthcheck of the cluster, and the Skipper process receives the TERM signal, it won't exit by itself
immediately, but will start reporting failures on healthcheck requests. Until it gets killed by the kubelet,
Skipper keeps serving the requests in this case.

Example - Ingress

A basic ingress specification:

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with ratelimiting

The example shows 50 calls per minute are allowed to each skipper
instance for the given ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/ratelimit: ratelimit(50, "1m")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with client based ratelimiting

The example shows 3 calls per minute per client, based on
X-Forwarded-For header or IP incase there is no X-Forwarded-For header
set, are allowed to each skipper instance for the given ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/ratelimit: localRatelimit(3, "1m")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

The example shows 500 calls per hour per client, based on
Authorization header set, are allowed to each skipper instance for the
given ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/ratelimit: localRatelimit(500, "1h", "auth")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with custom skipper filter configuration

The example shows the use of 2 filters from skipper for the implicitly
defined route in ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-filter: localRatelimit(50, "10m") -> requestCookie("test-session", "abc")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with custom skipper Predicate configuration

The example shows the use of a skipper predicates for the implicitly
defined route in ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-predicate: QueryParam("query", "^example$")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

Example - Ingress with shadow traffic

This will send production traffic to app-default.example.org and
copies incoming requests to https://app.shadow.example.org, but drops
responses from shadow URL. This is helpful to test your next
generation software with production workload. See also
https://godoc.org/github.com/zalando/skipper/filters/tee for details.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-filter: tee("https://app.shadow.example.org")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

*/
package kubernetes
