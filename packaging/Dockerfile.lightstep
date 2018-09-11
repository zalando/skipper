FROM registry.opensource.zalan.do/stups/alpine:latest
MAINTAINER Skipper Maintainers <team-pathfinder@zalando.de>
RUN apk --no-cache add ca-certificates && update-ca-certificates
RUN mkdir -p /usr/bin
ADD skipper eskip /usr/bin/
RUN mkdir -p /plugins
ADD build/tracing_lightstep.so /plugins/

EXPOSE 9090 9911

ENTRYPOINT ["/usr/bin/skipper", "-plugindir", "/plugins"]
