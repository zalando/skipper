FROM golang:buster

WORKDIR /workspace

COPY . .

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y make clang-11 && \
    go install github.com/mdempsky/go114-fuzz-build@latest && \
    apt-get autoremove --purge -y && apt-get autoclean && \
    go mod init fuzz && go mod tidy && \
    make all CC=clang-11 -j $(nproc)

ENTRYPOINT [ "/usr/bin/make" ]

CMD ["fuzz", "FUZZ_TARGETS=FuzzParseEskip"]
