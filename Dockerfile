FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:6f2f384d6467e8416ef84336f51b949767a2f9b51d4ea8365ae8d5962c7ccbb8
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
