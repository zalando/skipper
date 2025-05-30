FROM amazonlinux:2023@sha256:cb5b4c509d62ae388f674c139ae5e8281fc160c217d474445e912043e1941988

WORKDIR /workspace

ENV PATH="/root/go/bin:$PATH"

COPY . .

RUN dnf install -q -y make clang tar gzip       && \
    update-alternatives --install /usr/bin/cc cc /usr/bin/clang 20 && \
    curl -o go.linux-amd64.tar.gz -LO https://go.dev/dl/go1.22.1.linux-amd64.tar.gz && \
    sha256sum go.linux-amd64.tar.gz | grep -q aab8e15785c997ae20f9c88422ee35d962c4562212bb0f879d052a35c8307c7f && \
    tar -C /root -xzf go.linux-amd64.tar.gz && \
    ls -l /root && \
    go version && \
    go install github.com/mdempsky/go114-fuzz-build@v0.0.0-20200813011514-561a369ae6e1 && \
    go mod init fuzz && \
    go mod tidy && \
    make -s -j $(nproc) && \
    dnf autoremove -q -y && \
    dnf clean all -q && \
    go clean -cache -testcache -modcache -fuzzcache

ENTRYPOINT [ "/usr/bin/make", "fuzz" ]

CMD ["FUZZ_TARGETS=FuzzParseEskip"]
