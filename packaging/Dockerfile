ARG BASE_IMAGE=default
FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:462214f352333771339bc11412cd7ae48ddeeeca0034f902c30b6aedb781b0fe AS default
FROM ${BASE_IMAGE}
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"
RUN apk --no-cache add ca-certificates && update-ca-certificates
RUN mkdir -p /usr/bin
ARG BUILD_FOLDER=build
ARG TARGETPLATFORM
ADD ${BUILD_FOLDER}/${TARGETPLATFORM}/skipper \
    ${BUILD_FOLDER}/${TARGETPLATFORM}/eskip \
    ${BUILD_FOLDER}/${TARGETPLATFORM}/webhook \
    ${BUILD_FOLDER}/${TARGETPLATFORM}/routesrv /usr/bin/
ENV PATH $PATH:/usr/bin

EXPOSE 9090 9911

CMD ["/usr/bin/skipper"]
