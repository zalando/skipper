#! /usr/bin/env bash
# Local load test for physicsShedder. Runs two scenarios back-to-back:
#   1) healthy  — fast inline backend, latency ~0ms
#   2) slow     — latency("100ms") injected, 2x the shedder's 50ms latencyTarget
#
# Both run with physicsShedder in "logInactive" mode so you can observe R,
# mu, sigma, threshold, pReject in the proxy log without 503s being served.
#
# Usage:
#   ./benchmark-shedder.sh [duration_seconds] [concurrency] [skipper-binary]
# Requires: ab (apachebench).

set -euo pipefail

duration=${1:-20}
concurrency=${2:-32}
bin=${3:-}

cwd=$(cd "$(dirname "$0")" && pwd)
if [ -z "$bin" ]; then
    bin=$(mktemp)
    echo "building skipper -> $bin"
    (cd "$cwd/.." && go build -o "$bin" ./cmd/skipper)
fi

which ab >/dev/null 2>&1 || { echo "ab (apachebench) required"; exit 2; }

log_dir=$(mktemp -d)
echo "logs: $log_dir"

cleanup() {
    [ -n "${BPID:-}" ] && kill "$BPID" 2>/dev/null || true
    [ -n "${PPID_SK:-}" ] && kill "$PPID_SK" 2>/dev/null || true
}
trap cleanup EXIT INT

run_scenario() {
    local name=$1
    local proxy_eskip=$2

    echo
    echo "=== scenario: $name ==="
    "$bin" -access-log-disabled -address :9980 \
        -routes-file "$cwd/physics-shedder-backend.eskip" -support-listener :0 \
        > "$log_dir/backend-$name.log" 2>&1 &
    BPID=$!
    "$bin" -access-log-disabled -address :9090 \
        -routes-file "$cwd/$proxy_eskip" -support-listener :0 \
        > "$log_dir/proxy-$name.log" 2>&1 &
    PPID_SK=$!
    sleep 1

    ab -t "$duration" -c "$concurrency" -q http://127.0.0.1:9090/ 2>&1 \
        | grep -E "Requests per|Time per|Failed|Complete|Non-2xx" || true

    sleep 1
    echo "--- last 5 physicsShedder ticks ---"
    grep -i "physicsshedder\[local\]" "$log_dir/proxy-$name.log" | tail -5 || true

    kill "$BPID" "$PPID_SK" 2>/dev/null || true
    wait "$BPID" "$PPID_SK" 2>/dev/null || true
    BPID=""; PPID_SK=""
    sleep 1
}

run_scenario healthy physics-shedder.eskip
run_scenario slow    physics-shedder-slow.eskip

echo
echo "full logs at $log_dir"
