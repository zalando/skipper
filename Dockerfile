FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:e17223daa3910c8590105db8fe94ec48499ddaa3fc3ad2f4a94f4e26d62473a6
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
