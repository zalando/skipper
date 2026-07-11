#! /bin/bash

# Benchmarks load balancer algorithms end to end against a large number
# of backend processes, to measure the algorithm overhead and lock
# contention under sustained load.
#
# The backends are okserver processes (see okserver.c), each responding
# with a fixed 200 on its own port. One skipper instance load balances a
# single route across all of them, and the load generator drives it for
# the configured duration, once per algorithm.
#
# usage: benchmark-lb-algorithms.sh [backends] [duration] [connections] [algorithms] [skipper-binary]
#
# example: benchmark-lb-algorithms.sh 1000 2m 128 "roundRobin weightedRoundRobin"
#
# The load is generated with wrk (https://github.com/wg/wrk), or hey
# (https://github.com/rakyll/hey) when wrk is not available.
#
# environment:
#   PROXYPORT          proxy listen port, default 9090
#   BASEPORT           first backend port, default 10000
#   PORTS_PER_PROCESS  backend ports served by one okserver process,
#                      default 1; raise it on systems where the process
#                      limit does not allow one process per backend

set -o pipefail

cwd="$(cd "$(dirname "$0")" && pwd)"
cd "$cwd" || exit 1

backends=${1:-1000}
duration=${2:-1m}
connections=${3:-128}
algorithms=${4:-"roundRobin random powerOfRandomNChoices weightedRoundRobin"}
bin=${5:-"$cwd"/../bin/skipper}

baseport=${BASEPORT:-10000}
proxyport=${PROXYPORT:-9090}

log() {
	echo "$@" >&2
}

if [ ! -x "$bin" ]; then
	log "skipper binary not found at $bin, run 'make skipper' first or pass the binary as the 5th argument"
	exit 1
fi

loadgen=
if command -v wrk > /dev/null; then
	loadgen=wrk
elif command -v hey > /dev/null; then
	loadgen=hey
else
	log "ERR: neither 'wrk' (https://github.com/wg/wrk) nor 'hey' (https://github.com/rakyll/hey) found in \$PATH"
	exit 2
fi

ulimit -n 65536 2> /dev/null

if [ ! -x okserver ] || [ okserver.c -nt okserver ]; then
	log [building okserver]
	cc -O2 -o okserver okserver.c -lpthread || exit 1
fi

backendpids=
skipperpid=

cleanup() {
	[ -n "$skipperpid" ] && kill -9 "$skipperpid" 2> /dev/null
	[ -n "$backendpids" ] && kill -9 $backendpids 2> /dev/null
	rm -f "$cwd"/lb-algorithms.eskip
}

trap 'cleanup; exit 0' SIGINT

portsperprocess=${PORTS_PER_PROCESS:-1}

log; log "[starting $backends backends]"
for ((i = 0; i < backends; i += portsperprocess)); do
	n=$portsperprocess
	[ $((i + n)) -gt "$backends" ] && n=$((backends - i))
	./okserver $((baseport + i)) $n &
	backendpids="$backendpids $!"
	disown
done

# wait for the last backend to accept connections
for ((i = 0; i < 50; i++)); do
	curl -s -o /dev/null http://127.0.0.1:$((baseport + backends - 1))/ && break
	sleep 0.1
done
log "[backends started]"

endpoints() {
	for ((i = 0; i < backends; i++)); do
		printf '"http://127.0.0.1:%d"' $((baseport + i))
		[ $i -lt $((backends - 1)) ] && printf ', '
	done
}

eps=$(endpoints)

run_load() {
	if [ "$loadgen" = wrk ]; then
		wrk -c "$connections" -d "$1" --latency http://127.0.0.1:$proxyport/
	else
		hey -z "$1" -c "$connections" http://127.0.0.1:$proxyport/ |
			grep -E 'Requests/sec|Average|Slowest|Fastest|50%|90%|99%|responses'
	fi
}

for algorithm in $algorithms; do
	echo "lb: * -> <$algorithm, $eps>;" > "$cwd"/lb-algorithms.eskip

	"$bin" -access-log-disabled -address ":$proxyport" \
		-routes-file "$cwd"/lb-algorithms.eskip -insecure \
		-support-listener :0 \
		-passive-health-check "period=5s,min-requests=10,max-drop-probability=0.9" \
		-idle-conns-num "$connections" -close-idle-conns-period=60s 2> \
		>(grep -Ev 'INFO|write: broken pipe|connection reset by peer') &
	skipperpid=$!
	disown

	for ((i = 0; i < 50; i++)); do
		curl -s -o /dev/null http://127.0.0.1:$proxyport/ && break
		sleep 0.1
	done
	if ! curl -s -o /dev/null http://127.0.0.1:$proxyport/; then
		log "ERR: skipper did not start listening on :$proxyport"
		cleanup
		exit 1
	fi

	log; log "[warmup $algorithm]"
	run_load 5s > /dev/null 2>&1
	log "[benchmarking $algorithm, $backends backends, $connections connections, $duration]"
	run_load "$duration"
	log "[benchmarking $algorithm done]"

	kill -9 "$skipperpid" 2> /dev/null
	skipperpid=
	sleep 0.5
done

cleanup
log; log [all done]
