[![Build Status](https://travis-ci.org/zalando/skipper.svg)](https://travis-ci.org/zalando/skipper)
[![Doc](https://img.shields.io/badge/-userdocs-darkblue.svg)](https://opensource.zalando.com/skipper)
[![GoDoc](https://godoc.org/github.com/zalando/skipper?status.svg)](https://godoc.org/github.com/zalando/skipper)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/zalando/skipper)](https://goreportcard.com/report/zalando/skipper)
[![codecov](https://codecov.io/gh/zalando/skipper/branch/master/graph/badge.svg)](https://codecov.io/gh/zalando/skipper)
[![GitHub release](https://img.shields.io/github/release/zalando/skipper.svg)](https://github.com/zalando/skipper/releases)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2461/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2461)

<p><img height="180" alt="Skipper" src="https://raw.githubusercontent.com/zalando/skipper/master/img/skipper-h180.png"></p>

# Skipper

Skipper is an HTTP router and reverse proxy for service composition. It's designed to handle >300k HTTP route
definitions with detailed lookup conditions, and flexible augmentation of the request flow with filters. It can be
used out of the box or extended with custom lookup, filter logic and configuration sources.

## Main features:

An overview of [deployments and data-clients](https://opensource.zalando.com/skipper/operation/deployment/)
shows some use cases to run skipper.

Skipper

- identifies routes based on the requests' properties, such as path, method, host and headers
- allows modification of the requests and responses with filters that are independently configured for each route
- simultaneously streams incoming requests and backend responses
- optionally acts as a final endpoint (shunt), e.g. as a static file server or a mock backend for diagnostics
- updates routing rules without downtime, while supporting multiple types of data sources — including
  [etcd](https://github.com/coreos/etcd), [Kubernetes Ingress](https://opensource.zalando.com/skipper/data-clients/kubernetes/), [Innkeeper (deprecated)](https://github.com/zalando/innkeeper), [static files](https://opensource.zalando.com/skipper/data-clients/eskip-file/), [route string](https://opensource.zalando.com/skipper/data-clients/route-string/) and
  [custom configuration sources](https://godoc.org/github.com/zalando/skipper/predicates/#source)
- can serve as a
  [Kubernetes Ingress controller](https://zalando.github.io/skipper/data-clients/kubernetes/)
  without reloads. You can use it in combination with a controller that will route public traffic to
  your skipper fleet; [see AWS example](https://github.com/zalando-incubator/kube-ingress-aws-controller)
- shipped with eskip: a descriptive configuration language designed for routing rules

Skipper provides a default executable command with a few built-in filters. However, its primary use case is to
be extended with custom filters, predicates or data sources. [Go here for additional documentation](https://godoc.org/github.com/zalando/skipper).

A few examples for extending Skipper:

- Image server https://github.com/zalando-stups/skrop
- Plugins repository https://github.com/skipper-plugins/, [plugin docs](https://opensource.zalando.com/skipper/reference/plugins/)

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

    GO111MODULE=on go get github.com/zalando/skipper/...


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

To run `eskip` you first mount the `.eskip` file, into the container, and run the command

    docker run \
      -v $(PWD)/doc-docker-intro.eskip:/doc-docker-intro.eskip \
      registry.opensource.zalan.do/pathfinder/skipper:latest eskip print doc-docker-intro.eskip

To run `skipper` you first mount the `.eskip` file, into the container, expose the ports and run the command

    docker run -it \
        -v $(PWD)/doc-docker-intro.eskip:/doc-docker-intro.eskip \
        -p 9090:9090 \
        -p 9911:9911 \
        registry.opensource.zalan.do/pathfinder/skipper:latest skipper -routes-file doc-docker-intro.eskip

Skipper will then be available on http://localhost:9090

#### Authentication Proxy

Skipper can be used as an authentication proxy, to check incoming
requests with Basic auth or an OAuth2 provider including audit
logging. See the documentation at:
[https://godoc.org/github.com/zalando/skipper/filters/auth](https://godoc.org/github.com/zalando/skipper/filters/auth).


#### Working with the code

Working with the code requires Go1.11 or a higher version. Getting the code with the test dependencies (`-t` switch):

    GO111MODULE=on go get -t github.com/zalando/skipper/...

Build and test all packages:

    cd src/github.com/zalando/skipper
    make deps
    make install
    make shortcheck

> On Mac the tests may fail because of low max open file limit. Please make sure you have correct limits setup
by following [these instructions](https://gist.github.com/tombigel/d503800a282fcadbee14b537735d202c).

##### Working from IntelliJ / GoLand

To run or debug skipper from _IntelliJ IDEA_ or _GoLand_, you need to create this configuration:

| Parameter         | Value                                    |
|-------------------|------------------------------------------|
| Template          | Go Build                                 |
| Run kind          | Directory                                |
| Directory         | skipper source dir + `/cmd/skipper`      |
| Working directory | skipper source dir (usually the default) |

#### Kubernetes Ingress

Skipper can be used to run as an Kubernetes Ingress controller.
[Details with examples](https://opensource.zalando.com/skipper/data-clients/kubernetes)
of [Skipper's capabilities](https://opensource.zalando.com/skipper/data-clients/kubernetes/#why-to-choose-skipper) and an
[overview](https://opensource.zalando.com/skipper/operation/deployment/#kubernetes-ingress)
you will can be found in our [ingress-controller deployment docs](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/).

For AWS integration, we provide an ingress controller
https://github.com/zalando-incubator/kube-ingress-aws-controller, that
manage ALBs in front of your skipper deployment.
A production example,
https://github.com/zalando-incubator/kubernetes-on-aws/blob/dev/cluster/manifests/skipper/daemonset.yaml,
can be found in our Kubernetes configuration https://github.com/zalando-incubator/kubernetes-on-aws.

### Documentation

[Skipper's Documentation](https://opensource.zalando.com/skipper) and
[Godoc developer documentation](https://godoc.org/github.com/zalando/skipper),
includes information about [deployment use cases](https://opensource.zalando.com/skipper/operation/deployment/)
and detailed information on these topics:

- The [Routing](https://godoc.org/github.com/zalando/skipper/routing) Mechanism
- Matching Requests
- [Filters](https://opensource.zalando.com/skipper/reference/filters/) - Augmenting Requests and Responses
- Service Backends
- Route Definitions
- Data Sources: [eskip file](https://godoc.org/github.com/zalando/skipper/eskipfile), [etcd](https://godoc.org/github.com/zalando/skipper/etcd), [Kubernetes](https://godoc.org/github.com/zalando/skipper/dataclients/kubernetes), [Route string](https://godoc.org/github.com/zalando/skipper/dataclients/routestring)
- [Circuit Breakers](https://godoc.org/github.com/zalando/skipper/filters/circuit)
- Extending It with Customized [Predicates](https://opensource.zalando.com/skipper/reference/predicates/), [Filters](https://opensource.zalando.com/skipper/reference/filters/), can be done by [Plugins](https://opensource.zalando.com/skipper/reference/plugins/) or [Lua Scripts](https://opensource.zalando.com/skipper/reference/scripts/)
- [Predicates](https://opensource.zalando.com/skipper/reference/predicates/) - additional predicates to match a route
- [Proxy Packages](https://godoc.org/github.com/zalando/skipper/proxy)
- [Logging](https://godoc.org/github.com/zalando/skipper/logging) and [Metrics](https://godoc.org/github.com/zalando/skipper/metrics)
- Performance Considerations
- [Rate Limiters](https://godoc.org/github.com/zalando/skipper/reference/filters/#ratelimit)
- [Opentracing plugin](https://github.com/skipper-plugins/opentracing/) or extend [create your own](https://opensource.zalando.com/skipper/reference/plugins/#opentracing-plugins)

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

This introduction was [moved to ingress controller documentation](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/#install-skipper-as-ingress-controller).

For More details, please check out our [Kubernetes ingress controller docs](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/), our [ingress usage](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/) and how to handle [common backend problems in Kubernetes](https://opensource.zalando.com/skipper/kubernetes/ingress-backends/).

You should have a base understanding of [Kubernetes](https://kubernetes.io) and
[Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/).

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

We also have a slack channel #skipper in gophers.slack.com. Get an invite at [gophers official invite page](https://invite.slack.golangbridge.org).

### Proposals

We do our proposals open in [Skipper's Google drive](https://drive.google.com/drive/folders/0B9LwJMF9koB-ZEk4bEhZal9uOWM).
If you want to make a proposal feel free to create an
[issue](https://github.com/zalando/skipper/issues) and if it is a
bigger change we will invite you to a document, such that we can work together.

### Users

Zalando uses this project as shop frontend http router with 350000 routes, as
Kubernetes ingress controller and runs several custom skipper
instances that use skipper as library.

Sergio Ballesteros from [spotahome](https://www.spotahome.com/)
> We also ran tests with several ingress controllers and skipper gave us the more reliable results. Currently we are running skipper since almost 2 years with like 20K Ingress rules.
> The fact that skipper is written in go let us understand the code, add features and fix bugs since all of our infra stack is golang.

#### In the media

Blog posts:

- [Building our own open source http routing
  solution](https://jobs.zalando.com/tech/blog/building-our-own-open-source-http-routing-solution/):
  Giving some context about why Skipper was created in the first place.
- [Kubernetes in production @ ShopGun](https://itnext.io/kubernetes-in-production-shopgun-2c280f0c0923)
- Hacker News [Skipper – An HTTP router and reverse proxy for service composition](https://news.ycombinator.com/item?id=18837936)

Conference/Meetups talks

- [LISA 2018 - modern HTTP routing](https://www.usenix.org/conference/lisa18/presentation/szucs)
