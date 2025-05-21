#! /bin/bash
set -eo pipefail

if [ $# -lt 2 ]; then
    echo "usage: $0 <version> <sha512sum>"
    exit 1
fi

ETCD_VERSION="$1"
ETCD_CHECKSUM="$2"
ETCD_URL="https://github.com/etcd-io/etcd/releases/download/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz"

LOCAL_GOBIN=

if [ -n "${GOBIN}" ]; then
    LOCAL_GOBIN="$GOBIN"
elif [ -n "${GOPATH}" ]; then
    LOCAL_GOBIN="$GOPATH/bin"
fi

mkdir -p .bin

curl -LsSfo ./.bin/etcd.tar.gz "${ETCD_URL}"

echo ${ETCD_CHECKSUM} ./.bin/etcd.tar.gz | sha512sum --check - 

tar -xzf .bin/etcd.tar.gz --strip-components=1 \
    -C ./.bin "etcd-${ETCD_VERSION}-linux-amd64/etcd"

if [ -n "$LOCAL_GOBIN" ]; then
    echo installing etcd in "$LOCAL_GOBIN"
    mkdir -p "$LOCAL_GOBIN"
    cp ./.bin/etcd "$LOCAL_GOBIN"
fi
