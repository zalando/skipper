FROM registry.opensource.zalan.do/stups/alpine:latest
LABEL maintainer="Team Teapot @ Zalando SE <team-teapot@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
