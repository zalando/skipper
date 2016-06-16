FROM registry.opensource.zalan.do/pathfinder/archlinux:latest-SNAPSHOT
MAINTAINER Skipper Maintainers <team-pathfinder@zalando.de>
RUN pacman --noconfirm -Syyu
RUN pacman --noconfirm -S git go
RUN mkdir -p /opt/go-workspace
ENV GOPATH /opt/go-workspace
ENV PATH $PATH:$GOPATH/bin
RUN go get github.com/zalando/skipper/...
