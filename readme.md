[![Build Status](https://github.com/zalando/skipper/actions/workflows/master.yaml/badge.svg)](https://github.com/zalando/skipper/actions/workflows/master.yaml)
[![Doc](https://img.shields.io/badge/user-documentation-darkblue.svg)](https://opensource.zalando.com/skipper)
[![Go Reference](https://pkg.go.dev/badge/github.com/zalando/skipper.svg)](https://pkg.go.dev/github.com/zalando/skipper)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/zalando/skipper)](https://goreportcard.com/report/zalando/skipper)
[![Coverage Status](https://coveralls.io/repos/github/zalando/skipper/badge.svg?branch=master)](https://coveralls.io/github/zalando/skipper?branch=master)
[![GitHub release](https://img.shields.io/github/release/zalando/skipper.svg)](https://github.com/zalando/skipper/releases)
[![OpenSSF Best Practices](https://bestpractices.coreinfrastructure.org/projects/2461/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2461)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/zalando/skipper/badge)](https://api.securityscorecards.dev/projects/github.com/zalando/skipper)
[![Slack](https://img.shields.io/badge/Gopher%20Slack-%23skipper-green.svg)](https://invite.slack.golangbridge.org/)
![CodeQL](https://github.com/zalando/skipper/actions/workflows/codeql-analysis.yml/badge.svg)



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
  [etcd](https://github.com/coreos/etcd), [Kubernetes Ingress](https://opensource.zalando.com/skipper/data-clients/kubernetes/), [static files](https://opensource.zalando.com/skipper/data-clients/eskip-file/), [route string](https://opensource.zalando.com/skipper/data-clients/route-string/) and
  [custom configuration sources](https://opensource.zalando.com/skipper/tutorials/development/#dataclients)
- can serve as a
  [Kubernetes Ingress controller](https://zalando.github.io/skipper/data-clients/kubernetes/)
  without reloads. You can use it in combination with a controller that will route public traffic to
  your skipper fleet; [see AWS example](https://github.com/zalando-incubator/kube-ingress-aws-controller)
- shipped with
   - eskip: a descriptive configuration language designed for routing
     rules
   - routesrv: proxy to omit kube-apiserver overload leveraging Etag
     header to reduce amount of CPU used in your skipper data plane
   - webhook: Kubernetes validation webhook to make sure your
     manifests are deployed safely

Skipper provides a default executable command with a few built-in filters. However, its primary use case is to
be extended with custom filters, predicates or data sources. [Go here for additional documentation](https://pkg.go.dev/github.com/zalando/skipper).

A few examples for extending Skipper:

- Example proxy with custom filter https://github.com/szuecs/skipper-example-proxy
- Image server https://github.com/zalando-stups/skrop
- Plugins repository https://github.com/skipper-plugins/, [plugin docs](https://opensource.zalando.com/skipper/reference/plugins/)

### Getting Started

#### Prerequisites/Requirements

In order to build and run Skipper, only the latest version of Go needs to be installed. Skipper can use
Innkeeper or Etcd as data sources for routes, or for the simplest cases, a local configuration file. See more
details in the documentation: https://pkg.go.dev/github.com/zalando/skipper


#### Installation

##### From Binary

Download binary tgz from https://github.com/zalando/skipper/releases/latest

Example, assumes that you have $GOBIN set to a directory that exists
and is in your $PATH:

```
% curl -LO https://github.com/zalando/skipper/releases/download/v0.14.8/skipper-v0.14.8-linux-amd64.tar.gz
% tar xzf skipper-v0.14.8-linux-amd64.tar.gz
% mv skipper-v0.14.8-linux-amd64/* $GOBIN/
% skipper -version
Skipper version v0.14.8 (commit: 95057948, runtime: go1.19.1)
```

##### From Source


```
% git clone https://github.com/zalando/skipper.git
% make
% ./bin/skipper -version
Skipper version v0.14.8 (commit: 95057948, runtime: go1.19.3)
```

#### Running

Create a file with a route:

    echo 'hello: Path("/hello") -> "https://www.example.org"' > example.eskip

Optionally, verify the file's syntax:

    eskip check example.eskip

If no errors are detected nothing is logged, else a descriptive error is logged.

Start Skipper and make an HTTP request:

    skipper -routes-file example.eskip &
    curl localhost:9090/hello

##### Docker

To run the latest Docker container:

    docker run registry.opensource.zalan.do/teapot/skipper:latest

To run `eskip` you first mount the `.eskip` file, into the container, and run the command

    docker run \
      -v $(PWD)/doc-docker-intro.eskip:/doc-docker-intro.eskip \
      registry.opensource.zalan.do/teapot/skipper:latest eskip print doc-docker-intro.eskip

To run `skipper` you first mount the `.eskip` file, into the container, expose the ports and run the command

    docker run -it \
        -v $(PWD)/doc-docker-intro.eskip:/doc-docker-intro.eskip \
        -p 9090:9090 \
        -p 9911:9911 \
        registry.opensource.zalan.do/teapot/skipper:latest skipper -routes-file doc-docker-intro.eskip

Skipper will then be available on http://localhost:9090

#### Authentication Proxy

Skipper can be used as an authentication proxy, to check incoming
requests with Basic auth or an OAuth2 provider or an OpenID Connect
provider including audit logging. See the documentation at:
[https://pkg.go.dev/github.com/zalando/skipper/filters/auth](https://pkg.go.dev/github.com/zalando/skipper/filters/auth).


#### Working with the code

Getting the code with the test dependencies (`-t` switch):

    git clone https://github.com/zalando/skipper.git
    cd skipper

Build and test all packages:

    make deps
    make install
    make lint
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
manage ALBs or NLBs in front of your skipper deployment.
A [production example for skipper](https://github.com/zalando-incubator/kubernetes-on-aws/blob/stable/cluster/manifests/skipper/)
and a [production example for kube-ingress-aws-controller](https://github.com/zalando-incubator/kubernetes-on-aws/tree/dev/cluster/manifests/ingress-controller/),
can be found in our Kubernetes configuration https://github.com/zalando-incubator/kubernetes-on-aws.

- [Comparison with other Ingress controllers](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/#comparison-with-other-ingress-controllers)
- [Comparison with service-mesh](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/#comparison-with-service-mesh)

### Documentation

[Skipper's Documentation](https://opensource.zalando.com/skipper) and
[Godoc developer documentation](https://pkg.go.dev/github.com/zalando/skipper),
includes information about [deployment use cases](https://opensource.zalando.com/skipper/operation/deployment/)
and detailed information on these topics:

- The [Routing](https://pkg.go.dev/github.com/zalando/skipper/routing) Mechanism
- [Matching Requests](https://opensource.zalando.com/skipper/tutorials/basics/#route-matching)
- [Filters](https://opensource.zalando.com/skipper/reference/filters/) - Augmenting Requests and Responses
- [Predicates](https://opensource.zalando.com/skipper/reference/predicates/) - additional predicates to match a route
- Service [Backends](https://opensource.zalando.com/skipper/reference/backends/)
- Route Definitions fetched by dataclients:
   - [route string](https://opensource.zalando.com/skipper/data-clients/route-string/)
   - [eskip file](https://opensource.zalando.com/skipper/data-clients/eskip-file/)
   - [remote eskip](https://opensource.zalando.com/skipper/data-clients/eskip-remote/)
   - [etcd](https://opensource.zalando.com/skipper/data-clients/etcd/)
   - [kubernetes](https://opensource.zalando.com/skipper/data-clients/kubernetes/)
- [Circuit Breakers](https://pkg.go.dev/github.com/zalando/skipper/filters/circuit)
- Extending It with Custom [Predicates](https://opensource.zalando.com/skipper/tutorials/development/#predicates), [Filters](https://opensource.zalando.com/skipper/tutorials/development/#filters), can be done by [building your own proxy](https://opensource.zalando.com/skipper/tutorials/built-your-own/), [Plugins](https://opensource.zalando.com/skipper/reference/plugins/) or [Lua Scripts](https://opensource.zalando.com/skipper/reference/scripts/)
- [Proxy Package](https://pkg.go.dev/github.com/zalando/skipper/proxy)
- [Logging](https://pkg.go.dev/github.com/zalando/skipper/logging) and [Metrics](https://pkg.go.dev/github.com/zalando/skipper/metrics)
- [Operations guide](https://opensource.zalando.com/skipper/operation/operation/)
- [Authentication and Authorization](https://opensource.zalando.com/skipper/reference/filters/#authentication-and-authorization)
- [Load Shedders](https://opensource.zalando.com/skipper/reference/filters/#load-shedding)
- [Rate Limiters](https://pkg.go.dev/github.com/zalando/skipper/filters/ratelimit)
- [Opentracing tracers](https://pkg.go.dev/github.com/zalando/skipper/tracing/tracers) or extend [create your own](https://opensource.zalando.com/skipper/reference/plugins/#opentracing-plugins)

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

- baidu is using `Path()` matching to differentiate the HTTP requests to select the route.
- google is the default matching with wildcard `*`
- yandex is the default matching with wildcard `*` if you have a cookie `yandex=true`

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

To see the shadow traffic request that is made by the `tee()` filter you can use nc:

    [terminal1]% nc -l 12345
    [terminal2]% curl -v --cookie "yandex=true" localhost:9090/

#### 3 Minutes Skipper in Kubernetes introduction

This introduction was [moved to ingress controller documentation](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/#install-skipper-as-ingress-controller).

For More details, please check out our [Kubernetes ingress controller docs](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/), our [ingress usage](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/) and how to handle [common backend problems in Kubernetes](https://opensource.zalando.com/skipper/kubernetes/ingress-backends/).

### Packaging support

See https://github.com/zalando/skipper/blob/master/packaging/readme.md

In case you want to implement and link your own modules into your
skipper, there is https://github.com/skipper-plugins organization to
enable you to do so. In order to explain you the build process with
custom Go modules there is
https://github.com/skipper-plugins/skipper-tracing-build, that was
used to build skipper's [opentracing package](https://github.com/skipper-plugins/opentracing).
We moved the opentracing plugin source into the `tracing` package, so
there is no need to use plugins for this case.

Because Go plugins are not very well supported by Go itself we do not
recommend to use plugins, but you can extend skipper and
[build your own proxy](https://opensource.zalando.com/skipper/tutorials/built-your-own/).

## Community

User or developer questions can be asked in our [public Google Group](https://groups.google.com/forum/#!forum/skipper-router)

We have a slack channel #skipper in gophers.slack.com. Get an [invite](https://invite.slack.golangbridge.org).
If for some reason this link doesn't work, you can find more information about
the gophers communities [here](https://github.com/gobridge/about-us/blob/master/README.md#onlineoffline-communities).

The preferred communication channel is the slack channel, because the google group is a manual process to add members.
Feel also free to [create an issue](https://github.com/zalando/skipper/issues/new/choose), if you dislike chat and post your questions there.

### Proposals

We do our proposals open in [Skipper's Google drive](https://drive.google.com/drive/folders/0B9LwJMF9koB-ZEk4bEhZal9uOWM).
If you want to make a proposal feel free to create an
[issue](https://github.com/zalando/skipper/issues) and if it is a
bigger change we will invite you to a document, such that we can work together.

### Users

Zalando used this project as shop frontend http router with 350000 routes.
We use it as Kubernetes ingress controller in more than 100 production clusters. With every day traffic between 500k and 7M RPS serving 15000 ingress and 3750 RouteGroups at less than ¢5/1M requests.
We also run several custom skipper instances that use skipper as library.

Sergio Ballesteros from [spotahome](https://www.spotahome.com/) said 2018:
> We also ran tests with several ingress controllers and skipper gave us the more reliable results. Currently we are running skipper since almost 2 years with like 20K Ingress rules.
> The fact that skipper is written in go let us understand the code, add features and fix bugs since all of our infra stack is golang.

#### In the media

Blog posts:

- [opensource.com - Try this Kubernetes HTTP router and reverse proxy](https://opensource.com/article/20/4/http-kubernetes-skipper)
- [opensource.com - An open source HTTP router to increase your network visibility](https://opensource.com/article/20/5/skipper)
- [Building our own open source http routing
  solution](https://jobs.zalando.com/tech/blog/building-our-own-open-source-http-routing-solution/):
  Giving some context about why Skipper was created in the first place.
- [Kubernetes in production @ ShopGun](https://itnext.io/kubernetes-in-production-shopgun-2c280f0c0923)
- Hacker News [Skipper – An HTTP router and reverse proxy for service composition](https://news.ycombinator.com/item?id=18837936)

Conference/Meetups talks

- [LISA 2018 - modern HTTP routing](https://www.usenix.org/conference/lisa18/presentation/szucs)

## Version promise

Skipper will update the minor version in case we have either:

- a significant change
- a Go version requirement change (`go` directive in go.mod change)
- a dependency change that adds or removes a `replace` directive in
  go.mod file (requires library users to add or remove the same
  directive in their go.mod file)
- a change that require attention to users, for example Kubernetes
  RBAC changes required to deploy
  https://github.com/zalando/skipper/releases/tag/v0.18.0
- a feature removal like Kubernetes ingress v1beta1
  https://github.com/zalando/skipper/releases/tag/v0.15.0
- an API change of a function that is marked *experimental* [example](https://github.com/zalando/skipper/blob/e8c099f1740e3d85be0784d449b1177a48247813/io/read_stream.go#L209)

We expect that skipper library users will use
`skipper.Run(skipper.Options{})` as main interface that we do not want
to break. Besides the Kubernetes v1beta1 removal there was never a
change that removed an option. We also do not want to break generic
useful packages like `net`. Sometimes we mark library functions, that
we expect to be useful as *experimental*, because we want to try and
learn over time if this is a good API decision or if this limits us.

This promise we hold considering the main, filter, predicate,
dataclient, eskip interfaces and generic packages. For other packages,
we have more weak promise with backwards compatibility as these are
more internal packages. We try to omit breaking changes also in
internal packages. If this would mean too much work or impossible
to build new functionality as we would like, we will do a breaking
change considering strictly semantic versioning rules.

### How to update

Every update that changes the minor version (the `m` in `v0.m.p`),
should be done by `+1` only. So `v0.N.x` to `v0.N+1.y` and you should
read `v0.N+1.0` release page to see what can break and what you have
to do in order to have no issues while updating.
