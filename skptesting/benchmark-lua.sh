#! /bin/bash

if [ "$1" == -help ]; then
	log benchmark-lua.sh [duration] [connections] [warmup-duration]
	exit 0
fi

source $GOPATH/src/github.com/zalando/skipper/skptesting/benchmark.inc

trap cleanup SIGINT

log; log [starting servers]
skp :9990 redir.eskip
skp :9991 redir-lua.eskip
skp :9992 strip-query.eskip
skp :9993 strip-query-lua.eskip
log [servers started, wait 1 sec]
sleep 1

log; log [warmup]
warmup :9990
warmup :9991
warmup :9992
warmup :9993
log [warmup done]

log; log [benchmarking skipper-redirectTo]
bench :9990
log [benchmarking skipper-redirectTo done]

log; log [benchmarking redirect-lua]
bench :9991
log [benchmarking redirect-lua done]

log; log [benchmarking skipper-strip]
bench :9992
log [benchmarking skipper-strip done]

log; log [benchmarking strip-lua]
bench :9991
log [benchmarking strip-lua done]
cleanup

log; log [all done]
