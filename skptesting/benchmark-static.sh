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
ngx nginx-static.conf
skp :9990 static.eskip
log [servers started, wait 1 sec]
sleep 1

log; log [warmup]
warmup :9980
warmup :9990
log [warmup done]

log; log [benchmarking nginx]
bench :9980
log [benchmarking nginx done]

log; log [benchmarking skipper]
bench :9990
log [benchmarking skipper done]

cleanup
log; log [all done]
