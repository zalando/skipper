#! /bin/bash

if [ "$1" == -help ]; then
	echo benchmark-tokeninfo.sh [duration [connections [warmup-duration]]]
	exit 0
fi

source $GOPATH/src/github.com/zalando/skipper/skptesting/benchmark.inc

check_deps

trap cleanup-exit SIGINT

log; log [starting servers]
skp :9999 tokeninfo.eskip
skp :9080 ok.eskip
skp :9090 auth-proxy.eskip
log [servers started, wait 1 sec]
sleep 1

log; log [warmup]
warmup :9999
warmup :9080
warmup :9090 "Authorization:\ Bearer\ foo"
log [warmup done]

log; log [benchmarking baseline]
bench :9080
log [benchmarking baseline done]

log; log [benchmarking auth]
bench :9090
log [benchmarking auth done]

cleanup
log; log [all done]
