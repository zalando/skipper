FROM alpine
MAINTAINER Skipper Maintainers <team-pathfinder@zalando.de>
RUN apk add --update --no-cache ca-certificates
RUN mkdir -p /usr/bin
ADD skipper eskip /usr/bin/
ENV PATH $PATH:/usr/bin

CMD ["/usr/bin/skipper"]
