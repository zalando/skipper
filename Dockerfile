FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:3876b0971fe135f9ec92370025be828d55b411e2fc7c57691176ca02931647b3
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
