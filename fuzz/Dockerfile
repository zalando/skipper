FROM amazonlinux:2023@sha256:860843fc1dcf21cb06ce7a5cd0a6da92e9cb76bb8e0d1517fd93b6b8e2fa31bc

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
