FROM amazonlinux:2023@sha256:0aa4560b47da98d20a47b82c4185180dd7e76b7db5bdd145283d3fc7dfb52e9f

WORKDIR /workspace

ENV PATH="/root/go/bin:$PATH"

COPY . .

RUN dnf install -q -y make clang golang && \
    update-alternatives --install /usr/bin/cc cc /usr/bin/clang 20 && \
    go install github.com/mdempsky/go114-fuzz-build@v0.0.0-20200813011514-561a369ae6e1 && \
    go mod init fuzz && \
    go mod tidy && \
    make -s -j $(nproc) && \
    dnf autoremove -q -y && \
    dnf clean all -q && \
    go clean -cache -testcache -modcache -fuzzcache

ENTRYPOINT [ "/usr/bin/make", "fuzz" ]

CMD ["FUZZ_TARGETS=FuzzParseEskip"]
