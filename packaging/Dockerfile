ARG BASE_IMAGE=default
ARG BUILD_FOLDER=build
ARG TARGETPLATFORM

FROM registry.opensource.zalan.do/library/alpine-3:latest AS default
FROM ${BASE_IMAGE}
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

RUN cat /etc/alpine-release \
    && apk --no-cache add ca-certificates \
    && update-ca-certificates

COPY ${BUILD_FOLDER}/${TARGETPLATFORM}/skipper \
    ${BUILD_FOLDER}/${TARGETPLATFORM}/eskip \
    ${BUILD_FOLDER}/${TARGETPLATFORM}/webhook \
    ${BUILD_FOLDER}/${TARGETPLATFORM}/routesrv \
    /usr/bin/

EXPOSE 9090 9911

CMD ["/usr/bin/skipper"]
