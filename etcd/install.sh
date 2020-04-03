#! /bin/bash
set -eo pipefail

ETCD_VERSION=master
if [  $# -gt 0 ]; then
    ETCD_VERSION="$1"
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
tar -xzf .bin/etcd.tar.gz --strip-components=1 \
    -C ./.bin "etcd-${ETCD_VERSION}-linux-amd64/etcd"

if [ -n "$LOCAL_GOBIN" ]; then
    echo installing etcd in "$LOCAL_GOBIN"
    mkdir -p "$LOCAL_GOBIN"
    cp ./.bin/etcd "$LOCAL_GOBIN"
fi
