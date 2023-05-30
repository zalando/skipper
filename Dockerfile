FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:29c534fa01c053db96fc84b34162187d42e55f73728ca6f875fd59fd929633c5
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
