FROM golang:buster

WORKDIR /workspace

COPY . .

ENV DEBIAN_FRONTEND=noninteractive \
    CC=clang

RUN apt-get update && apt-get install -y make clang && \
    go install github.com/mdempsky/go114-fuzz-build@latest && \
    apt-get autoremove --purge -y && apt-get autoclean && \
    go mod init fuzz && go mod tidy && \
    /usr/bin/make all

ENTRYPOINT [ "/usr/bin/make" ]

CMD ["fuzz", "FUZZ_TARGETS=FuzzParseEskip"]
