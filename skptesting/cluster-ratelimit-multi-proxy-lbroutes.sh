#! /bin/bash

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
skipper -access-log-disabled -address ":9000" -inline-routes "r: * -> inlineContent(\"OK\") -> status(200) -> <shunt>;" &
pids="$pids $!"
skipper -access-log-disabled -address ":9001" -inline-routes "r: * -> inlineContent(\"OK\") -> status(200) -> <shunt>;" &
pids="$pids $!"

# fake ingress controller swarm
skipper -address :9091 -enable-ratelimits -enable-swarm \
        -swarm-static-self=127.0.0.1:9891 -swarm-static-other=127.0.0.1:9892 \
        -access-log-disabled -proxy-preserve-host \
        -routes-file "$cwd"/lb-with-ratelimit.eskip &
pids="$pids $!"
skipper -address :9092 -enable-ratelimits -enable-swarm \
        -swarm-static-self=127.0.0.1:9892 -swarm-static-other=127.0.0.1:9891 \
        -access-log-disabled -proxy-preserve-host \
        -routes-file "$cwd"/lb-with-ratelimit.eskip &
pids="$pids $!"

# fake alb with lb group of ingress skipper
skipper -access-log-disabled -proxy-preserve-host \
        -inline-routes 'r1: Traffic(0.5) -> "http://127.0.0.1:9091";
r2: * -> "http://127.0.0.1:9092";' &
pids="$pids $!"
log [servers started, wait 1 sec]

# validate setup
sleep 1
res=$(curl -s -H"Host: test.example.org" http://127.0.0.1:9090/)
if [ "$res" != "OK" ]
then
  log [setup not ready after 1s, quit]
  cleanup
  exit 1
fi
# log; log [test http requests]
# echo "GET http://127.0.0.1:9090/" | vegeta attack -header="Host: test.example.org" -rate=100 -duration=3s | vegeta report
# cooldown=11
# log [wait ${cooldown}s to cool down]
# sleep ${cooldown}

# test
d=120
rate=50
log; log [start the measurement requests]
for i in {0..5}; do
  echo "GET http://127.0.0.1:9090/" | vegeta attack -header="Host: foo.example.org" -rate=$rate -duration=${d}s | tee results.bin | vegeta report
done

cat <<EOF
results should be:
# 10 req/s allowed
# $rate req/s for ${d}s
# $(($rate * $d)) req in total
# $(($d * 10)) requests should be allowed and $(($rate * $d - $d * 10)) not allowed
EOF

cleanup
log; log [all done]
