FROM public.ecr.aws/docker/library/golang:1.21.0-bookworm as builder

RUN mkdir /app
WORKDIR /app

COPY go.mod go.sum /app/
RUN go mod tidy \
    && go mod download

COPY . /app

RUN CGO_ENABLED=1 \
    go \
      build \
      -trimpath \
      -buildmode=plugin \
      -o plugins/filters/teapot/teapot.so \
      plugins/filters/teapot/*.go

RUN CGO_ENABLED=1 \
    go \
      build \
      -trimpath \
      -o bin/skipper \
      ./cmd/skipper

FROM scratch

COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /lib/aarch64-linux-gnu/libc.so.6 /lib/aarch64-linux-gnu/libc.so.6
COPY --from=builder /lib/ld-linux-aarch64.so.1 /lib/ld-linux-aarch64.so.1

COPY --from=builder /app/bin/skipper /bin/skipper
COPY --from=builder /app/plugins/filters/teapot/teapot.so /plugins/filters/teapot.so

ENTRYPOINT ["/bin/skipper"]