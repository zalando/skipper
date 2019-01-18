#! /bin/bash

# build go test binaries
go build -o gohttpclient gohttpclient.go
#go build -o gorilla-ws-example gorilla-ws-example.go


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
skipper -experimental-upgrade -address :9091 -proxy-preserve-host -routes-file "$cwd"/lb_wrong.eskip \
	-idle-conns-num 2 -close-idle-conns-period 3 &
pids="$pids $!"

# fake alb
skipper -experimental-upgrade -access-log-disabled -proxy-preserve-host -idle-conns-num 20 -close-idle-conns-period 30 -inline-routes 'r: * -> "http://127.0.0.1:9091";' &
pids="$pids $!"
log [servers started, wait 1 sec]

sleep 1

#log; log [start connections wrk]
#wrk -H "Host: test.example.org"  -c "5" -d "12s" http://127.0.0.1:9090/  &

SEC=8
log; log [start connections vegeta]
#echo "GET http://127.0.0.1:9090/" | vegeta attack -header="Host: test.example.org" -output=vegeta_out.bin -rate=2 -duration=${SEC}s -http2=false &

# sleep $(($SEC / 4))

log; log [start ws]
# printf "GET /graphql-ws HTTP/1.1
# X-Forwarded-For: 185.85.220.255
# X-Forwarded-Proto: https
# X-Forwarded-Port: 443
# Host: beetroot-dev.fs-apps-test.zalan.do
# X-Amzn-Trace-Id: Root=1-5c45ab64-8f6822d81de4bc1635fabde2
# Upgrade: websocket
# Connection: upgrade
# Pragma: no-cache
# Cache-Control: no-cache
# User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/71.0.3578.98 Safari/537.36
# Origin: https://beetroot-dev.fs-apps-test.zalan.do
# Sec-WebSocket-Version: 13
# Accept-Encoding: gzip, deflate, br
# Accept-Language: en-US,en;q=0.9,ar-EG;q=0.8,ar;q=0.7
# Sec-WebSocket-Key: OKtbYQC4i4zFRmNDWw1egQ==
# Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits
# Sec-WebSocket-Protocol: graphql-ws" | nc 127.0.0.1 9090 &

# printf "GET /graphql-ws HTTP/1.1
# X-Forwarded-For: 185.85.220.255
# X-Forwarded-Proto: http
# X-Forwarded-Port: 80
# Host: wrong.example.org
# X-Amzn-Trace-Id: Root=1-5c45ab64-8f6822d81de4bc1635fabde2
# Upgrade: websocket
# Connection: upgrade
# Pragma: no-cache
# Cache-Control: no-cache
# User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/71.0.3578.98 Safari/537.36
# Origin: http://wrong.example.org
# Sec-WebSocket-Version: 13
# Accept-Encoding: gzip, deflate, br
# Accept-Language: en-US,en;q=0.9,ar-EG;q=0.8,ar;q=0.7
# Sec-WebSocket-Key: OKtbYQC4i4zFRmNDWw1egQ==
# Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits
# Sec-WebSocket-Protocol: graphql-ws" | nc 127.0.0.1 9090 &

# log; log [checking connections]
# sleep 1
# printf "GET / HTTP/1.1\r\nHost: test.example.org\r\n\r\n" | nc 127.0.0.1 9090 &
# echo ""
# sleep 1
# printf "GET / HTTP/1.1\r\nHost: test.example.org\r\n\r\n" | nc 127.0.0.1 9090 &
# echo ""
# sleep 1

#wrk -H "Host: test.example.org"  -c "5" -d "3" http://127.0.0.1:9090/
#slow :9090 "Host: test.example.org"

./gohttpclient &
clipid="$!"
# sleep 3
# ./gorilla-ws-example -addr=wrong.example.org:9090 &
# clipid="$clipid $!"

log [benchmarking skipper done]

sleep $SEC
# cat vegeta_out.bin | vegeta report

kill -INT $clipid

cleanup
log; log [all done]
