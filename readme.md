[![Build Status](https://travis-ci.org/zalando/skipper.svg)](https://travis-ci.org/zalando/skipper)
[![GoDoc](https://godoc.org/github.com/zalando/skipper?status.svg)](https://godoc.org/github.com/zalando/skipper)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/zalando/skipper)](https://goreportcard.com/report/zalando/skipper)
[![codecov](https://codecov.io/gh/zalando/skipper/branch/master/graph/badge.svg)](https://codecov.io/gh/zalando/skipper)

<p align="center"><img height="360" alt="Skipper" src="https://raw.githubusercontent.com/zalando/skipper/gh-pages/img/skipper.h360.png"></p>


# Skipper

Skipper is an HTTP router and reverse proxy for service composition. It's designed to handle >100k HTTP route
definitions with detailed lookup conditions, and flexible augmentation of the request flow with filters. It can be
used out of the box or extended with custom lookup, filter logic and configuration sources.

### NOTE for Skoap users

The Skoap filters can be found currently in the branch called 'skoap-migration'. The original incubator repository at zalando-incubator/skoap has been removed.

## Main features:

An overview of [deployments and data-clients](https://zalando.github.io/skipper/deployments/)
shows some use cases to run skipper.

Skipper

- identifies routes based on the requests' properties, such as path, method, host and headers
- allows modification of the requests and responses with filters that are independently configured for each route
- simultaneously streams incoming requests and backend responses
- optionally acts as a final endpoint (shunt), e.g. as a static file server or a mock backend for diagnostics
- updates routing rules without downtime, while supporting multiple types of data sources — including
  [etcd](https://github.com/coreos/etcd), [Kubernetes Ingress](https://zalando.github.io/skipper/dataclients/kubernetes/), [Innkeeper](https://github.com/zalando/innkeeper), [static files](https://zalando.github.io/skipper/dataclients/eskip-file/), [route string](https://zalando.github.io/skipper/dataclients/route-string/) and
  [custom configuration sources](https://godoc.org/github.com/zalando/skipper/predicates/source)
- can serve as a
  [Kubernetes Ingress controller](https://zalando.github.io/skipper/dataclients/kubernetes/)
  without reloads. You can use it in combination with a controller that will route public traffic to
  your skipper fleet; [see AWS example](https://github.com/zalando-incubator/kube-ingress-aws-controller)
- shipped with eskip: a descriptive configuration language designed for routing rules

Skipper provides a default executable command with a few built-in filters. However, its primary use case is to
be extended with custom filters, predicates or data sources. [Go here for additional documentation](https://godoc.org/github.com/zalando/skipper).

A few examples for extending Skipper:

- Authentication proxy https://github.com/zalando-incubator/skoap (repository removed see 'skoap-migration' branch)
- Image server https://github.com/zalando-stups/skrop


### Getting Started

#### Prerequisites/Requirements

In order to build and run Skipper, only the latest version of Go needs to be installed. Skipper can use
Innkeeper or Etcd as data sources for routes, or for the simplest cases, a local configuration file. See more
details in the documentation: https://godoc.org/github.com/zalando/skipper.


#### Installation

Skipper is 'go get' compatible. If needed, create a Go workspace first:

    mkdir ws
    cd ws
    export GOPATH=$(pwd)
    export PATH=$PATH:$GOPATH/bin

Get the Skipper packages:

    go get github.com/zalando/skipper/...


#### Running

Create a file with a route:

    echo 'hello: Path("/hello") -> "https://www.example.org"' > example.eskip

Optionally, verify the file's syntax:

    eskip check example.eskip

Start Skipper and make an HTTP request:

    skipper -routes-file example.eskip &
    curl localhost:9090/hello

##### Docker

To run the latest Docker container:

    docker run registry.opensource.zalan.do/pathfinder/skipper:latest

#### Working with the code

Getting the code with the test dependencies (`-t` switch):

    go get -t github.com/zalando/skipper/...

Build and test all packages:

    cd src/github.com/zalando/skipper
    make deps
    make install
    make check


#### Kubernetes Ingress

Skipper can be used to run as an Kubernetes Ingress controller.
[Details with examples](https://zalando.github.io/skipper/dataclients/kubernetes)
of [Skipper's capabilities](https://zalando.github.io/skipper/dataclients/kubernetes/#skipper-features) and an
[overview](https://zalando.github.io/skipper/deployments/#kubernetes-ingress)
you will can be found in our [deployment docs](https://zalando.github.io/skipper).

For AWS integration, we provide an ingress controller
https://github.com/zalando-incubator/kube-ingress-aws-controller, that
manage ALBs in front of your skipper deployment.
A production example,
https://github.com/zalando-incubator/kubernetes-on-aws/blob/dev/cluster/manifests/skipper/daemonset.yaml,
can be found in our Kubernetes configuration https://github.com/zalando-incubator/kubernetes-on-aws.

### Documentation

[Skipper's Documentation](https://zalando.github.io/skipper) and
[Godoc developer documentation](https://godoc.org/github.com/zalando/skipper),
includes information about [deployment use cases](https://zalando.github.io/skipper/deployments/)
and detailed information on these topics:

- The [Routing](https://godoc.org/github.com/zalando/skipper/routing) Mechanism
- Matching Requests
- [Filters](https://godoc.org/github.com/zalando/skipper/filters) - Augmenting Requests and Responses
- Service Backends
- Route Definitions
- Data Sources: [eskip file](https://godoc.org/github.com/zalando/skipper/eskipfile), [etcd](https://godoc.org/github.com/zalando/skipper/etcd), [Inkeeper API](https://godoc.org/github.com/zalando/skipper/innkeeper), [Kubernetes](https://godoc.org/github.com/zalando/skipper/dataclients/kubernetes), [Route string](https://godoc.org/github.com/zalando/skipper/dataclients/routestring)
- [Circuit Breakers](https://godoc.org/github.com/zalando/skipper/filters/circuit)
- Extending It with Customized Predicates, Filters, and Builds
- [Predicates](https://godoc.org/github.com/zalando/skipper/predicates) - additional predicates to match a route
- [Proxy Packages](https://godoc.org/github.com/zalando/skipper/proxy)
- [Logging](https://godoc.org/github.com/zalando/skipper/logging) and [Metrics](https://godoc.org/github.com/zalando/skipper/metrics)
- Performance Considerations
- [Rate Limiters](https://godoc.org/github.com/zalando/skipper/filters/ratelimit)

#### 1 Minute Skipper introduction

The following example shows a skipper routes file in eskip format, that has 3 named routes: baidu, google and yandex.

    % cat doc-1min-intro.eskip
    baidu:
            Path("/baidu")
            -> setRequestHeader("Host", "www.baidu.com")
            -> setPath("/s")
            -> setQuery("wd", "godoc skipper")
            -> "http://www.baidu.com";
    google:
            *
            -> setPath("/search")
            -> setQuery("q", "godoc skipper")
            -> "https://www.google.com";
    yandex:
            * && Cookie("yandex", "true")
            -> setPath("/search/")
            -> setQuery("text", "godoc skipper")
            -> tee("http://127.0.0.1:12345/")
            -> "https://yandex.ru";

Matching the route:

- baidu is using Path() matching to differentiate the HTTP requests to select the route.
- google is the default matching with wildcard '*'
- yandex is the default matching with wildcard '*' if you have a cookie "yandex=true"

Request Filters:

- If baidu is selected, skipper sets the Host header, changes the path and sets a query string to the http request to the backend "http://www.baidu.com".
- If google is selected, skipper changes the path and sets a query string to the http request to the backend "https://www.google.com".
- If yandex is selected, skipper changes the path and sets a query string to the http request to the backend "https://yandex.ru". The modified request will be copied to "http://127.0.0.1:12345/"

Run skipper with the routes file doc-1min-intro.eskip shown above

    % skipper -routes-file doc-1min-intro.eskip

To test each route you can use curl:

    % curl -v localhost:9090/baidu
    % curl -v localhost:9090/
    % curl -v --cookie "yandex=true" localhost:9090/

To see the request that is made by the tee() filter you can use nc:

    [terminal1]% nc -l 12345
    [terminal2]% curl -v --cookie "yandex=true" localhost:9090/

#### 3 Minutes Skipper in Kubernetes introduction

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
      name: skipper-demo"
    spec:
      rules:
      - host: skipper-demo.<mydomain.org>
        http:
          paths:
          - backend:
              serviceName: skipper-demo"
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

### Packaging support

See https://github.com/zalando/skipper/blob/master/packaging/readme.md

In case you want to implement and link your own modules into your
skipper for more advanced features like [opentracing API](https://github.com/opentracing) support there
is https://github.com/skipper-plugins organization to enable you to do
so. In order to explain you the build process with custom Go modules
there is https://github.com/skipper-plugins/skipper-tracing-build,
that is used to build skipper's [opentracing package](https://github.com/skipper-plugins/opentracing).


## Community

User or developer questions can be asked in our [public Google Group](https://groups.google.com/forum/#!forum/skipper-router)

### Proposals

We do our proposals open in [Skipper's Google drive](https://drive.google.com/drive/folders/0B9LwJMF9koB-ZEk4bEhZal9uOWM).
If you want to make a proposal feel free to create an
[issue](https://github.com/zalando/skipper/issues) and if it is a
bigger change we will invite you to a document, such that we can work together.
