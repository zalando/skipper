FROM spearheads.mop.zalan.do/spearheads/ubuntu-go:latest
MAINTAINER Team Spearheads <team-spearheads@zalando.de>

ADD . /opt/go
WORKDIR /opt/go
ENV GOPATH /opt/go

RUN go install skipper

EXPOSE 9090

CMD /opt/go/bin/skipper -insecure -etcd-urls $ETCD_URL
