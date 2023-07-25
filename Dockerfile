FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:462214f352333771339bc11412cd7ae48ddeeeca0034f902c30b6aedb781b0fe
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
