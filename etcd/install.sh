#! /bin/bash

set -e 

ETCD_VERSION=master
if [  $# -gt 0 ]; then
    ETCD_VERSION="$1"
fi

mkdir -p $GOPATH/src/github.com/coreos
cd $GOPATH/src/github.com/coreos
git clone https://github.com/coreos/etcd.git || echo etcd repository exists
cd etcd
git checkout master
git pull
git checkout $ETCD_VERSION
rm -rf gopath
echo building etcd $ETCD_VERSION
./build
mkdir -p $GOPATH/bin
mv bin/etcd $GOPATH/bin
