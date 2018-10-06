# Introduction

This is the documentation page of [Skipper](https://github.com/zalando/skipper). Skipper is an HTTP router and reverse proxy for service composition. It's designed to handle >100k HTTP route definitions with detailed lookup conditions, and flexible augmentation of the request flow with filters. It can be used out of the box or extended with custom lookup, filter logic and configuration sources.

## HTTP Proxy

Skipper identifies routes based on the requests' properties, such as path, method, host and headers using the
[predicates](reference/predicates.md). It allows the modification of the requests and responses with
[filters](reference/filters.md) that are independently configured for each route. [See more here about how it
works.](reference/architecture.md)

## Kubernetes Ingress

Skipper can be used to run as a Kubernetes Ingress controller. Details with examples of Skipper's capabilities
and an overview you will can be found in the [ingress-controller deployment
docs](kubernetes/ingress-controller.md).
