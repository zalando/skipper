FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:d4f07233a12a9dcb6770e943b6d2a77f274dfcd18eafe075a3697601c2a68141
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
