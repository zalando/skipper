FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:2924ef858efe1025d8f4094b1e3ee6f86db2d3e211052c06e0c8afb86643d46a
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
