FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:698d5d39d8756bfe04723cc8bfe0aee3c55fe3896074a77dbed5db355b5695dc
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
