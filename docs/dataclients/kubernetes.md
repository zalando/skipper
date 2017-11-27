# Kubernetes

Skipper's Kubernetes dataclient can be used, if you want to run skipper as
[kubernetes-ingress-controller](https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-controllers).
It will get it's route information from provisioned
[Ingress Objects](https://kubernetes.io/docs/concepts/services-networking/ingress).
Detailed information you find in our [godoc for dataclient kubernetes](https://godoc.org/github.com/zalando/skipper/dataclients/kubernetes).

## Skipper Features

Skipper has the following main features:

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
  - [metrics](https://godoc.org/github.com/zalando/skipper/metrics)
  - access logs
  - Blue-Green deployments, with another Ingress annotation `zalando.org/backend-weights`, see Advanced Examples section

## 3 Minutes Skipper in Kubernetes introduction

You should have a base understanding of [Kubernetes](https://kubernetes.io) and
[Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/).

Prerequisites: First you have to install skipper-ingress as for
example daemonset, create a deployment and a service.

We start to deploy skipper-ingress as a daemonset, use hostNetwork and
expose the TCP port 9999 on each Kubernetes worker node for incoming ingress
traffic.

    % cat skipper-ingress-ds.yaml
    apiVersion: extensions/v1beta1
    kind: DaemonSet
    metadata:
      name: skipper-ingress
      namespace: kube-system
      labels:
        application: skipper-ingress
        version: v0.9.115
        component: ingress
    spec:
      selector:
        matchLabels:
          application: skipper-ingress
      updateStrategy:
        type: RollingUpdate
      template:
        metadata:
          name: skipper-ingress
          labels:
            application: skipper-ingress
            version: v0.9.115
            component: ingress
          annotations:
            scheduler.alpha.kubernetes.io/critical-pod: ''
        spec:
          affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                - matchExpressions:
                  - key: master
                    operator: DoesNotExist
          tolerations:
          - key: CriticalAddonsOnly
            operator: Exists
          hostNetwork: true
          containers:
          - name: skipper-ingress
            image: registry.opensource.zalan.do/pathfinder/skipper:v0.9.115
            ports:
            - name: ingress-port
              containerPort: 9999
              hostPort: 9999
            args:
              - "skipper"
              - "-kubernetes"
              - "-kubernetes-in-cluster"
              - "-address=:9999"
              - "-proxy-preserve-host"
              - "-serve-host-metrics"
              - "-enable-ratelimits"
              - "-experimental-upgrade"
              - "-metrics-exp-decay-sample"
            resources:
              limits:
                cpu: 200m
                memory: 200Mi
              requests:
                cpu: 25m
                memory: 25Mi
            readinessProbe:
              httpGet:
                path: /kube-system/healthz
                port: 9999
              initialDelaySeconds: 5
              timeoutSeconds: 5


We now deploy a simple demo application serving html:

    % cat demo-deployment.yaml
    apiVersion: apps/v1beta1
    kind: Deployment
    metadata:
      name: skipper-demo
    spec:
      replicas: 2
      template:
        metadata:
          labels:
            application: skipper-demo
        spec:
          containers:
          - name: skipper-demo
            image: registry.opensource.zalan.do/pathfinder/skipper:v0.9.117
            args:
              - "skipper"
              - "-inline-routes"
              - "* -> inlineContent(\"<body style='color: white; background-color: green;'><h1>Hello!</h1>\") -> <shunt>"
            ports:
            - containerPort: 9090

We deploy a service type ClusterIP that we will select from ingress:

    % cat demo-svc.yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: sszuecs-demo
      labels:
        application: skipper-demo
    spec:
      type: ClusterIP
      ports:
        - port: 80
          protocol: TCP
          targetPort: 9090
          name: external
      selector:
        application: sszuecs-demo

To deploy both, you have to run:

    % kubectl create -f demo-deployment.yaml
    % kubectl create -f demo-svc.yaml

Now we have a skipper-ingress running as daemonset exposing the TCP
port 9999 on each worker node, a backend application running with 2
replicas that serves some html on TCP port 9090, and we expose a
cluster service on TCP port 80. Besides skipper-ingress, deployment
and service can not be reached from outside the cluster. Now we expose
the application with Ingress to the external network:

    % cat demo-ing.yaml
    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      name: skipper-demo
    spec:
      rules:
      - host: skipper-demo.<mydomain.org>
        http:
          paths:
          - backend:
              serviceName: skipper-demo
              servicePort: 80

To deploy this ingress, you have to run:

    % kubectl create -f demo-ing.yaml

Skipper will configure itself for the given ingress, such that you can test doing:

    % curl -v -H"Host: skipper-demo.<mydomain.org>" http://<nodeip>:9999/

The next question you may ask is: how to expose this to your customers?

The answer depends on your setup and complexity requirements. In the
simplest case you could add one A record in your DNS *.<mydomain.org>
to your frontend loadbalancer IP that directs all traffic from *.<mydomain.org>
to all Kubernetes worker nodes on TCP port 9999.

A more complex setup we use in production and can be done with
something that configures your frontend loadbalancer, for example
[kube-aws-ingress-controller](https://github.com/zalando-incubator/kube-ingress-aws-controller),
and your DNS, [external-dns](https://github.com/kubernetes-incubator/external-dns)
automatically.

## Advanced Examples

### Blue-Green deployments

To do blue-green deployments you have to have control over traffic
switching. Skipper gives you the opportunity to set weights to backend
services in your ingress specification. `zalando.org/backend-weights`
is a hash map, which key relates to the `serviceName` of the backend
and the value is the weight of traffic you want to send to the
particular backend. It works for more than 2 backends, but for
simplicity this example shows 2 backends, which should be the default
case for supporting blue-green deployments.

In the following example **my-app-1** service will get **80%** of the traffic
and **my-app-2** will get **20%** of the traffic:

    apiVersion: extensions/v1beta1
    kind: Ingress
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

### Circuitbreaker

#### Consecutive Breaker

The [consecutiveBreaker](https://godoc.org/github.com/zalando/skipper/filters/circuit#NewConsecutiveBreaker)
filter is a breaker for the ingress route that open if the backend failures
for the route reach a value of N (in this example N=15), where N is a
mandatory argument of the filter and there are some more optional arguments
[documented](https://godoc.org/github.com/zalando/skipper/filters/circuit#NewConsecutiveBreaker):

    consecutiveBreaker(15)

The ingress spec would look like this:

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-filter: consecutiveBreaker(15)
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

#### Rate Breaker

The [rateBreaker](https://godoc.org/github.com/zalando/skipper/filters/circuit#NewRateBreaker)
filter is a breaker for the ingress route that open if the backend
failures for the route reach a value of N within a window of the last
M requests, where N (in this example 30) and M (in this example 300)
are mandatory arguments of the filter and there are some more optional arguments
[documented](https://godoc.org/github.com/zalando/skipper/filters/circuit#NewRateBreaker).

    rateBreaker(30, 300)

The ingress spec would look like this:

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-filter: rateBreaker(30, 300)
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80


### Ratelimits

More details you will find in [ratelimit package](https://godoc.org/github.com/zalando/skipper/filters/ratelimit)
and [kubernetes dataclient](https://godoc.org/github.com/zalando/skipper/dataclients/kubernetes) documentation.

#### Client Ratelimits

The example shows 3 calls per minute per client, based on
X-Forwarded-For header or IP incase there is no X-Forwarded-For header
set, are allowed to each skipper instance for the given ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-filter: localRatelimit(3, "1m")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80


#### Service Ratelimits

The example shows 50 calls per minute are allowed to each skipper
instance for the given ingress.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-filter: ratelimit(50, "1m")
      name: app
    spec:
      rules:
      - host: app-default.example.org
        http:
          paths:
          - backend:
              serviceName: app-svc
              servicePort: 80

### use Predicates

[Predicates](https://godoc.org/github.com/zalando/skipper/predicates)
are influencing the route matching, which you might want to carefully
test before using it in production. This enables you to do feature
toggles or time based enabling endpoints.

You can use all kinds of [predicates](https://godoc.org/github.com/zalando/skipper/predicates)
with [filters](https://godoc.org/github.com/zalando/skipper/filters) together.

#### Feature Toggle

This ingress route will only be matched if there is a Querystring
"version=alpha" defined in the request. Like this you can easily build
feature toggles.

    apiVersion: extensions/v1beta1
    kind: Ingress
    metadata:
      annotations:
        zalando.org/skipper-predicate: QueryParam("version", "^alpha$")
      name: alpha
    spec:
      rules:
      - host: alpha-default.example.org
        http:
          paths:
          - backend:
              serviceName: alpha-svc
              servicePort: 80


### Chaining Filters

You can set multiple filters in a chain similar to the [eskip format](https://godoc.org/github.com/zalando/skipper/eskip).

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
