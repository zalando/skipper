#! /bin/bash

# Benchmarks the UseHostTree routing option end-to-end against three scenarios:
#
#   1. 1 route           — baseline, minimal routing table
#   2. 10 000 routes,    — all HostAny("bench.test"), different Path predicates
#      same host           The matching route is a catch-all at the end.
#   3. 10 000 routes,    — each route has a unique HostAny and Path.
#      different hosts     3 routes match HostAny("bench.test"); the host tree
#                          reduces the candidate set to those 3.
#
# Each scenario is run twice: without and with -use-host-tree.
# The load generator sends all requests with "Host: bench.test".
#
# usage: benchmark-host-tree.sh [duration] [connections] [warmup-duration] [skipper-binary]
#
# example: skptesting/benchmark-host-tree.sh 12 128 3 ./bin/skipper
#
# Dependencies: bash, wrk (https://github.com/wg/wrk), a C compiler (for okserver).

set -o pipefail

cwd="$(cd "$(dirname "$0")" && pwd)"
cd "$cwd" || exit 1

d=${1:-12}
c=${2:-128}
wd=${3:-3}
bin=${4:-"$cwd"/../bin/skipper}

host_header="bench.test"
backendport=11000
proxyport=9191

log() { echo "$@" >&2; }

if [ ! -x "$bin" ]; then
    log "skipper binary not found at $bin, run 'make skipper' first or pass it as the 4th argument"
    exit 1
fi

if ! command -v wrk > /dev/null; then
    log "ERR: 'wrk' (https://github.com/wg/wrk) not found in \$PATH"
    exit 2
fi

if [ ! -x okserver ] || [ okserver.c -nt okserver ]; then
    log "[building okserver]"
    cc -O2 -o okserver okserver.c -lpthread || exit 1
fi

backendpid=
skipperpid=
tmpdir=$(mktemp -d)

cleanup() {
    [ -n "$skipperpid" ] && kill -9 "$skipperpid" 2>/dev/null
    [ -n "$backendpid" ] && kill -9 "$backendpid" 2>/dev/null
    rm -rf "$tmpdir"
}
trap 'cleanup; exit 0' SIGINT EXIT

# ── generate route files ────────────────────────────────────────────────────

routes_1="$tmpdir/routes-1.eskip"
cat > "$routes_1" <<'EOF'
r0: * -> <shunt>;
EOF

routes_same="$tmpdir/routes-10k-same-host.eskip"
{
    for i in $(seq 0 9998); do
        printf 'r%d: HostAny("bench.test") && Path("/r%d") -> <shunt>;\n' "$i" "$i"
    done
    echo 'r_last: * -> <shunt>;'
} > "$routes_same"

routes_diff="$tmpdir/routes-10k-diff-host.eskip"
{
    for i in $(seq 0 9999); do
        printf 'r%d: HostAny("host%d.test") && Path("/r%d") -> <shunt>;\n' "$i" "$i" "$i"
    done
    echo 'ra: HostAny("bench.test") && Path("/a") -> <shunt>;'
    echo 'rb: HostAny("bench.test") && Path("/b") -> <shunt>;'
    echo 'r_last: HostAny("bench.test") && Path("/") -> <shunt>;'
} > "$routes_diff"

# ── start backend ───────────────────────────────────────────────────────────

log; log "[starting okserver on :$backendport]"
./okserver "$backendport" &
backendpid=$!

for i in $(seq 1 50); do
    curl -s -o /dev/null "http://127.0.0.1:$backendport/" && break
    sleep 0.1
done

# ── helpers ─────────────────────────────────────────────────────────────────

start_skipper() {
    local routes_file="$1"
    local extra_flags="$2"
    "$bin" -access-log-disabled \
        -address ":$proxyport" \
        -routes-file "$routes_file" \
        -insecure \
        -support-listener :0 \
        -idle-conns-num "$c" \
        -close-idle-conns-period=3s \
        $extra_flags \
        2> >(grep -Ev 'INFO|write: broken pipe|connection reset by peer') &
    skipperpid=$!

    for i in $(seq 1 50); do
        curl -s -o /dev/null -H "Host: $host_header" "http://127.0.0.1:$proxyport/" && break
        sleep 0.1
    done
}

stop_skipper() {
    [ -n "$skipperpid" ] && kill -9 "$skipperpid" 2>/dev/null
    skipperpid=
    sleep 0.3
}

run_bench() {
    local label="$1"
    log; log "[$label — warmup ${wd}s]"
    wrk -c "$c" -d "${wd}s" -H "Host: $host_header" "http://127.0.0.1:$proxyport/" > /dev/null 2>&1
    log "[$label — measuring ${d}s]"
    echo "=== $label ==="
    wrk --latency -c "$c" -d "${d}s" -H "Host: $host_header" "http://127.0.0.1:$proxyport/"
}

# ── scenario 1: 1 route ──────────────────────────────────────────────────────

log; log "=== Scenario 1: 1 route ==="

start_skipper "$routes_1" ""
run_bench "1-route / no-host-tree"
stop_skipper

start_skipper "$routes_1" "-use-host-tree"
run_bench "1-route / use-host-tree"
stop_skipper

# ── scenario 2: 10 000 routes, same host ─────────────────────────────────────

log; log "=== Scenario 2: 10 000 routes, all HostAny(bench.test) ==="

start_skipper "$routes_same" ""
run_bench "10k-same-host / no-host-tree"
stop_skipper

start_skipper "$routes_same" "-use-host-tree"
run_bench "10k-same-host / use-host-tree"
stop_skipper

# ── scenario 3: 10 000 routes, different hosts ───────────────────────────────

log; log "=== Scenario 3: 10 000 routes, different hosts (3 match bench.test) ==="

start_skipper "$routes_diff" ""
run_bench "10k-diff-host / no-host-tree"
stop_skipper

start_skipper "$routes_diff" "-use-host-tree"
run_bench "10k-diff-host / use-host-tree"
stop_skipper

log; log "[all done]"
