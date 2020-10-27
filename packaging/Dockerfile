FROM alpine
LABEL maintainer="Team Teapot @ Zalando SE <team-teapot@zalando.de>"
RUN apk --no-cache add ca-certificates && update-ca-certificates
RUN mkdir -p /usr/bin
ADD skipper eskip webhook /usr/bin/
ENV PATH $PATH:/usr/bin

EXPOSE 9090 9911

CMD ["/usr/bin/skipper"]
