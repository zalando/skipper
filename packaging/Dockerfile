ARG BASE_IMAGE=default
FROM alpine:latest AS default
FROM ${BASE_IMAGE}
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

# https://docs.docker.com/engine/reference/builder/#automatic-platform-args-in-the-global-scope
ARG TARGETPLATFORM

RUN apk --no-cache add ca-certificates && update-ca-certificates

COPY build/${TARGETPLATFORM}/skipper \
    build/${TARGETPLATFORM}/eskip \
    build/${TARGETPLATFORM}/webhook \
    build/${TARGETPLATFORM}/routesrv \
    /usr/bin/

EXPOSE 9090 9911

CMD ["/usr/bin/skipper"]
