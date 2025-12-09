ARG BASE_IMAGE=default
FROM registry.opensource.zalan.do/library/alpine-3:latest AS default
FROM ${BASE_IMAGE}

ARG VERSION
ARG REVISION

LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"
LABEL org.opencontainers.image.title=skipper
LABEL org.opencontainers.image.description="An HTTP router and reverse proxy for service composition, including use cases like Kubernetes Ingress"
LABEL org.opencontainers.image.version=$VERSION
LABEL org.opencontainers.image.url=https://github.com/zalando/skipper
LABEL org.opencontainers.image.licenses=Apache-2.0
LABEL org.opencontainers.image.vendor="Zalando SE"
LABEL org.opencontainers.image.source=https://github.com/zalando/skipper
LABEL org.opencontainers.image.revision=$REVISION

RUN apk --no-cache add ca-certificates && update-ca-certificates
RUN mkdir -p /usr/bin
ARG BUILD_FOLDER=build
ARG TARGETPLATFORM
ADD ${BUILD_FOLDER}/${TARGETPLATFORM}/skipper \
    ${BUILD_FOLDER}/${TARGETPLATFORM}/eskip \
    ${BUILD_FOLDER}/${TARGETPLATFORM}/webhook \
    ${BUILD_FOLDER}/${TARGETPLATFORM}/routesrv /usr/bin/
ENV PATH=$PATH:/usr/bin

EXPOSE 9090 9911

CMD ["/usr/bin/skipper"]
