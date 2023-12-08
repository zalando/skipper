#! /bin/bash
set -eo pipefail

ETCD_VERSION=v3.5.11
if [ $# -gt 0 ]; then
    ETCD_VERSION="$1"
fi

ETCD_CHECKSUM=4fb304f384dd4d6e491e405fed8375a09ea1c6c2596b93f97cb31844202e620df160f87f18611e84f17675e7b7245e40d1aa23571ecdb507cb094ba04d378171
if [ $# -gt 1 ]; then
    ETCD_CHECKSUM="$2"
fi

LOCAL_GOBIN=
if [ -n "${GOBIN}" ]; then
    LOCAL_GOBIN="$GOBIN"
elif [ -n "${GOPATH}" ]; then
    LOCAL_GOBIN="$GOPATH/bin"
fi

mkdir -p .bin
wget \
    "https://github.com/etcd-io/etcd/releases/download/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz" \
    -O ./.bin/etcd.tar.gz

echo "${ETCD_CHECKSUM} ./.bin/etcd.tar.gz" | sha512sum -c

tar -xzf .bin/etcd.tar.gz --strip-components=1 \
    -C ./.bin "etcd-${ETCD_VERSION}-linux-amd64/etcd"

if [ -n "$LOCAL_GOBIN" ]; then
    echo installing etcd in "$LOCAL_GOBIN"
    mkdir -p "$LOCAL_GOBIN"
    cp ./.bin/etcd "$LOCAL_GOBIN"
fi
