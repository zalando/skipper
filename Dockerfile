FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:2213d4d74c39af5313b631cbde2630b4007755b280f0f6b98867f66103b76113
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
