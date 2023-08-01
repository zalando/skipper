FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:ac3a6db04df95d849533ced3ce5cb697dc042e53613866e27066cc471ed63070
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
