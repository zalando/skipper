FROM pierone.stups.zalan.do/mop-taskforce/ubuntu-go:1.4.2
MAINTAINER Team Spearheads <team-spearheads@zalando.de>

ADD . /opt/skipper
WORKDIR /opt/skipper
ENV GOPATH /opt/skipper

RUN go generate eskip && go install skipper

EXPOSE 9090

CMD /opt/skipper/bin/skipper -insecure -etcd-urls $ETCD_URL
