FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:30191d6129bb8709d59b4b198839bb1b877763693831f5c922e14bd8b23affc3
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
