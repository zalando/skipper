# Skipper Plugins for Opentracing API

This repository is the first plugin repository for
[skipper](https://github.com/zalando/skipper).

To build [skipper](https://github.com/zalando/skipper) with
[Opentracing](https://github.com/opentracing) plugins, see
[skipper tracing build](https://github.com/skipper-plugins/skipper-tracing-build) repository.

## Docker

A dockerized [skipper](https://github.com/zalando/skipper) with [Opentracing](https://github.com/opentracing) you can get with:

    % docker run registry.opensource.zalan.do/teapot/skipper-tracing:latest

## Problem with vendoring

Please note the problems that may arise when using plugins together with vendoring, best described here:

https://github.com/golang/go/issues/20481

In case of the opentracing plugins, the import path conflict will most certainly happen with vendoring because
the interface between the plugin and Skipper includes types from the github.com/opentracing/opentracing-go 3rd
party library.
