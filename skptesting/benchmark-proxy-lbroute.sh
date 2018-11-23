#! /bin/bash

if [ "$1" == -help ]; then
	log $0 [duration] [connections] [warmup-duration]
	exit 0
fi

source $GOPATH/src/github.com/zalando/skipper/skptesting/benchmark.inc

check_deps

trap cleanup-exit SIGINT

log; log [starting servers]

# 2 backends
skipper -access-log-disabled -address ":9000" -inline-routes "r: * -> inlineContent(\"A\") -> status(200) -> <shunt>;" &
pids="$pids $!"
skipper -access-log-disabled -address ":9001" -inline-routes "r: * -> inlineContent(\"B\") -> status(200) -> <shunt>;" &
pids="$pids $!"

# lb
skp :9090 lb.eskip
log [servers started, wait 1 sec]
sleep 1

log; log [warmup]
warmup :9090 "Host: test.example.org"
log [warmup done]

log; log [benchmarking skipper]
bench :9090 "Host: test.example.org"
log [benchmarking skipper done]

cleanup
log; log [all done]
