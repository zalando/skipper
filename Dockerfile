FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:1e1dfdf788f9f3e5b37626389d830e6bbd98c68dd5e43031102291363316cbbc
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
