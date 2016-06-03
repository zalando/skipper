#! /bin/bash

if [ "$1" == -help ]; then
	echo benchmark-compress.sh [duration] [warmup-duration]
	exit 0
fi

source $GOPATH/src/github.com/zalando/skipper/testing/benchmark.inc

trap cleanup SIGINT

echo [generating content]
lorem
echo [content generated]

echo; echo [starting servers]
ngx nginx-static.conf
ngx nginx-proxy.conf
skp :9090 proxy.eskip
echo [servers started, wait 1 sec]
sleep 3

echo; echo [warmup]
warmup :9980
warmup :9080
warmup :9090
echo [warmup done]

echo; echo [benchmarking nginx]
bench :9080 Accept-Encoding:\ gzip,deflate
echo [benchmarking nginx done]

echo; echo [benchmarking skipper]
bench :9090 Accept-Encoding:\ gzip,deflate
echo [benchmarking skipper done]

cleanup
echo; echo [all done]
