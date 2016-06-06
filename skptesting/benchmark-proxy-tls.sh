#! /bin/bash

if [ "$1" == -help ]; then
	log benchmark-proxy.sh [duration] [connections] [warmup-duration]
	exit 0
fi

source $GOPATH/src/github.com/zalando/skipper/skptesting/benchmark.inc

trap cleanup SIGINT

log [generating content]
lorem
log [content generated]

log; log [starting servers]
skp :9980 static.eskip tls
skp :9090 proxy-tls.eskip
log [servers started, wait 1 sec]
sleep 1

log; log [warmup]
warmup :9980 "" tls
warmup :9090
log [warmup done]

log; log [benchmarking skipper]
bench :9090
log [benchmarking skipper done]

cleanup
log; log [all done]
