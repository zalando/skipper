FROM alpine
MAINTAINER Skipper Maintainers <team-pathfinder@zalando.de>
RUN apk --no-cache add ca-certificates && update-ca-certificates
RUN mkdir -p /usr/bin
ADD skipper eskip /usr/bin/
ENV PATH $PATH:/usr/bin

CMD ["/usr/bin/skipper"]
