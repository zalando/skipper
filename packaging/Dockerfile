ARG BASE_IMAGE=registry.opensource.zalan.do/library/alpine-3:latest@sha256:2924ef858efe1025d8f4094b1e3ee6f86db2d3e211052c06e0c8afb86643d46a
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
