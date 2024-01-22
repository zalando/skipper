FROM amazonlinux:2023@sha256:fe46a60a46e7334e360e63125aea37b282f9992102e45b764459dc0a5ffcfafc

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
