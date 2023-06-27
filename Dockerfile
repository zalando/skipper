FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:cca6473d06047d18e164d06ffdacd84cd6d82f353ce6270b6afe00b149e39e89
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
