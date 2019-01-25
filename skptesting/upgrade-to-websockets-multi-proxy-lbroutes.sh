#! /bin/bash

# build go test binaries
go build -o gohttpclient gohttpclient.go

# shellcheck source=./benchmark.inc
source "$GOPATH/src/github.com/zalando/skipper/skptesting/benchmark.inc"

if [ "$1" == -help ] || [ "$1" == --help ] || [ "$1" == -h ]; then
	log "$0 [duration] [connections] [warmup-duration]"
	exit 0
fi

check_deps

trap cleanup-exit SIGINT

log; log [starting servers]

# 2 backends
skipper -experimental-upgrade -access-log-disabled -address ":9000" -inline-routes "r: * -> inlineContent(\"CORRECT\") -> status(200) -> <shunt>;" &
pids="$pids $!"
skipper -experimental-upgrade -access-log-disabled -address ":9001" -inline-routes "r: * -> inlineContent(\"WRONG\") -> status(200) -> <shunt>;" &
pids="$pids $!"

# lb
skipper -experimental-upgrade -address :9091 -proxy-preserve-host -routes-file "$cwd"/lb_group.eskip \
	-idle-conns-num 2 -close-idle-conns-period 3 &
pids="$pids $!"

# fake alb
skipper -experimental-upgrade -access-log-disabled -proxy-preserve-host -idle-conns-num 20 -close-idle-conns-period 30 -inline-routes 'r: * -> "http://127.0.0.1:9091";' &
pids="$pids $!"
log [servers started, wait 1 sec]

sleep 1

SEC=8
log; log [start http requests and upgrade to websocket call]

./gohttpclient &
pids="$pids $!"

sleep $SEC
log [skipper test done]

cleanup
log; log [all done]
