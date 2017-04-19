#!/bin/sh

if [  $# -gt 0 ]
    then
    ETCD_VERSION="$1";
    else
    ETCD_VERSION="master";
fi

echo "Using ETCD version $ETCD_VERSION"

mkdir /hack
git clone https://github.com/coreos/etcd.git /hack/etcd
cd /hack/etcd
git checkout $ETCD_VERSION
./build
