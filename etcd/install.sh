#! /bin/bash
set -eo pipefail

ETCD_VERSION=master
if [  $# -gt 0 ]; then
    ETCD_VERSION="$1"
fi

if [ -z "${GOBIN}" ]; then
    LOCAL_GOBIN="$GOPATH/bin"
else
    LOCAL_GOBIN="$GOBIN"
fi

mkdir -p "$LOCAL_GOBIN"
wget \
    "https://github.com/etcd-io/etcd/releases/download/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz" \
    -O /tmp/etcd.tar.gz
tar -xzf /tmp/etcd.tar.gz --strip-components=1 \
    -C "$LOCAL_GOBIN" "etcd-${ETCD_VERSION}-linux-amd64/etcd"
