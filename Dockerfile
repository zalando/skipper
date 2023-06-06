FROM registry.opensource.zalan.do/library/alpine-3:latest@sha256:3a5ed584d25bc240f33f6ba7fe61f50c05172f89b03a8764ea1a41937f04f727
LABEL maintainer="Team Gateway&Proxy @ Zalando SE <team-gwproxy@zalando.de>"

ADD skipper /usr/bin/

ENV PATH $PATH:/usr/bin

RUN mkdir plugins
CMD ["/usr/bin/skipper"]
