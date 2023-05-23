FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:1acc8ff68cc482f52a9d34950fa3e963c5854521aa11fe07b4341645c7e4484e
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
