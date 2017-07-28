#! /bin/bash

if [ "$1" == -help ]; then
	log benchmark-proxy.sh [duration] [connections] [warmup-duration]
	exit 0
fi

source $GOPATH/src/github.com/zalando/skipper/skptesting/benchmark.inc

trap cleanup-exit SIGINT

log [generating content]
lorem
log [content generated]

log; log [starting servers]
skp :9980 static.eskip
# ngx nginx-proxy.conf
skp :9090 proxy.eskip
log [servers started, wait 1 sec]
sleep 1

log; log [warmup]
warmup :9980
# warmup :9080
warmup :9090
log [warmup done]

# log; log [benchmarking nginx]
# bench :9080
# log [benchmarking nginx done]

log; log [benchmarking skipper]
bench :9090
log [benchmarking skipper done]

cleanup
log; log [all done]
