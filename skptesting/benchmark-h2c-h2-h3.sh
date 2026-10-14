#! /bin/bash
#
# benchmark-h2c-h2-h3.sh — benchmark skipper's h2c, h2, and h3 backend transports.
#
# Each test starts a purpose-built Go backend server, then starts skipper with the
# matching -enable-<protocol>-backend flag, then drives load from the client to
# skipper using wrk (HTTP/1.1) or hey as fallback.  The h2c/h2/h3 transport is
# exercised on the proxy→backend leg; the client→proxy leg is always HTTP/1.1.
#
# For h3, where the proxy→backend QUIC path is also visible, a separate
# benchmark-h3-loadgen run is offered as an additional h3 client-to-proxy check.
#
# Usage:
#   benchmark-h2c-h2-h3.sh [protocol] [duration] [connections] [warmup] [skipper-binary]
#
#   protocol       h2c | h2 | h3 | all   (default: all)
#   duration       benchmark duration in seconds (default: 12)
#   connections    concurrent connections (default: 128)
#   warmup         warmup duration in seconds (default: 3)
#   skipper-binary path to skipper binary (default: ../bin/skipper)
#
# Dependencies:
#   wrk  https://github.com/wg/wrk        (preferred load generator)
#   hey  https://github.com/rakyll/hey     (fallback)
#   go                                     (to compile backend server and h3 loadgen)
#   openssl                                (auto-generates cert.pem/key.pem when missing)

set -o pipefail

cwd="$(cd "$(dirname "$0")" && pwd)"
cd "$cwd" || exit 1

proto=${1:-all}
d=${2:-12}
c=${3:-128}
wd=${4:-3}
bin=${5:-"$cwd/../bin/skipper"}

log() { echo "$@" >&2; }

# ---------- dependency checks ------------------------------------------------

if [ ! -x "$bin" ]; then
    log "skipper binary not found at $bin, run 'make skipper' first or pass it as the 5th argument"
    exit 1
fi

if ! command -v go > /dev/null; then
    log "ERR: 'go' not found in \$PATH (required to build the backend server and h3 loadgen)"
    exit 2
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

# ---------- TLS cert (shared by h2 and h3) -----------------------------------

ensure_cert() {
    if [ ! -f "$cwd/cert.pem" ] || [ ! -f "$cwd/key.pem" ]; then
        log "[generating self-signed TLS cert]"
        if ! command -v openssl > /dev/null; then
            log "ERR: 'openssl' not found; cannot generate TLS cert (run: skptesting/self-cert.sh)"
            return 1
        fi
        openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
            -keyout "$cwd/key.pem" -out "$cwd/cert.pem" \
            -nodes -days 1 -subj "/CN=localhost" \
            -addext "subjectAltName=IP:127.0.0.1,DNS:localhost" 2>/dev/null
    fi
}

# ---------- lorem.html -------------------------------------------------------

# ensure_backend_bin pre-builds the backend server binary once so that repeated
# 'go run' calls during all/h2/h3 don't pay the compilation cost each time.
backendbin=
ensure_backend_bin() {
    if [ -z "$backendbin" ]; then
        backendbin=$(mktemp /tmp/benchmark-backend-server.XXXXXX)
        log "[building backend server binary]"
        go build -o "$backendbin" "$cwd/benchmark-backend-server.go" || {
            log "ERR: failed to build benchmark-backend-server.go"
            exit 1
        }
    fi
}

ensure_lorem() {
    if [ ! -f "$cwd/lorem.html" ]; then
        log "[generating lorem.html]"
        loremHead='<!doctype html><html><head><meta charset="utf-8"><title>Lorem</title></head><body>'
        loremP='<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.</p>'
        loremTail='</body></html>'
        printf '%s' "$loremHead" > "$cwd/lorem.html"
        for _ in $(seq 1 360); do printf '%s' "$loremP" >> "$cwd/lorem.html"; done
        printf '%s' "$loremTail" >> "$cwd/lorem.html"
    fi
}

# ---------- process management -----------------------------------------------

backendpid=
skipperpid=

# kill_tree kills a process and its direct children (one level deep).
# Avoids killing the script's own process group.
kill_tree() {
    local pid=$1
    [ -z "$pid" ] && return
    # kill children first, then parent
    pkill -9 -P "$pid" 2>/dev/null
    kill -9 "$pid" 2>/dev/null
    wait "$pid" 2>/dev/null
}

cleanup() {
    kill_tree "$skipperpid"
    kill_tree "$backendpid"
    skipperpid=
    backendpid=
    # give the OS a moment to release ports
    sleep 0.3
}

cleanup_all() {
    cleanup
    [ -n "$backendbin" ] && rm -f "$backendbin"
}

trap 'cleanup_all; exit 0' SIGINT SIGTERM

# ---------- wait helpers -----------------------------------------------------

# wait_tcp retries a curl health check until the server responds or times out.
# Passes -k to skip TLS verification for self-signed certs.
# Tries for up to 10s to accommodate 'go run' compilation on first invocation.
wait_tcp() {
    local url=$1
    for _ in $(seq 1 100); do
        curl -sfk -o /dev/null --max-time 0.5 "$url" 2>/dev/null && return 0
        sleep 0.1
    done
    log "ERR: server did not start at $url"
    return 1
}

# ---------- load runner (HTTP/1.1 client → proxy) ----------------------------

run_load() {
    local url=$1 dur=$2
    if [ "$loadgen" = wrk ]; then
        wrk -c "$c" -d "${dur}s" --latency "$url"
    else
        hey -z "${dur}s" -c "$c" "$url" |
            grep -E 'Requests/sec|Average|Slowest|Fastest|50%|90%|99%|responses'
    fi
}

# ---------- individual benchmarks --------------------------------------------

bench_h2c() {
    log; log "=== h2c backend benchmark ==="
    ensure_backend_bin

    log "[starting h2c backend on :9980]"
    "$backendbin" -proto h2c -addr :9980 2>/dev/null &
    backendpid=$!
    wait_tcp "http://127.0.0.1:9980/health" || { cleanup; return 1; }

    log "[starting skipper with -enable-h2c-backend on :9090]"
    "$bin" \
        -access-log-disabled \
        -address :9090 \
        -routes-file "$cwd/proxy-h2c-backend.eskip" \
        -enable-h2c-backend \
        -idle-conns-num "$c" \
        -close-idle-conns-period 3s \
        -support-listener :0 \
        2> >(grep -Ev 'INFO|write: broken pipe|connection reset by peer') &
    skipperpid=$!

    wait_tcp "http://127.0.0.1:9090/health" || { cleanup; return 1; }

    log "[warmup ${wd}s]"
    run_load "http://127.0.0.1:9090/lorem.html" "$wd" > /dev/null 2>&1
    log "[warmup done]"

    log "[benchmarking h2c ${d}s: wrk/hey → skipper(:9090) h2c → backend(:9980)]"
    run_load "http://127.0.0.1:9090/lorem.html" "$d"

    cleanup
    log "=== h2c done ==="
}

bench_h2() {
    log; log "=== h2 backend benchmark ==="
    ensure_backend_bin
    ensure_cert || return 1

    log "[starting h2 TLS backend on :9980]"
    "$backendbin" -proto h2 -addr :9980 \
        -cert "$cwd/cert.pem" -key "$cwd/key.pem" 2>/dev/null &
    backendpid=$!
    # health check over HTTPS (skipper verifies via -insecure, curl skips via -k)
    wait_tcp "https://127.0.0.1:9980/health" || { cleanup; return 1; }

    log "[starting skipper with -enable-h2-backend on :9090]"
    "$bin" \
        -access-log-disabled \
        -address :9090 \
        -routes-file "$cwd/proxy-h2-backend.eskip" \
        -enable-h2-backend \
        -insecure \
        -idle-conns-num "$c" \
        -close-idle-conns-period 3s \
        -support-listener :0 \
        2> >(grep -Ev 'INFO|write: broken pipe|connection reset by peer') &
    skipperpid=$!

    wait_tcp "http://127.0.0.1:9090/health" || { cleanup; return 1; }

    log "[warmup ${wd}s]"
    run_load "http://127.0.0.1:9090/lorem.html" "$wd" > /dev/null 2>&1
    log "[warmup done]"

    log "[benchmarking h2 ${d}s: wrk/hey → skipper(:9090) h2 TLS → backend(:9980)]"
    run_load "http://127.0.0.1:9090/lorem.html" "$d"

    cleanup
    log "=== h2 done ==="
}

bench_h3() {
    log; log "=== h3 backend benchmark ==="
    ensure_backend_bin
    ensure_cert || return 1

    log "[starting h3 QUIC backend on UDP :9980]"
    "$backendbin" -proto h3 -addr :9980 \
        -cert "$cwd/cert.pem" -key "$cwd/key.pem" 2>/dev/null &
    backendpid=$!
    # h3 backend speaks UDP; no TCP health check. Skipper will fail-fast on first request.
    sleep 0.5

    log "[starting skipper with -enable-h3-backend on :9090]"
    "$bin" \
        -access-log-disabled \
        -address :9090 \
        -routes-file "$cwd/proxy-h3-backend.eskip" \
        -enable-h3-backend \
        -insecure \
        -idle-conns-num "$c" \
        -close-idle-conns-period 3s \
        -support-listener :0 \
        2> >(grep -Ev 'INFO|write: broken pipe|connection reset by peer') &
    skipperpid=$!

    wait_tcp "http://127.0.0.1:9090/health" || { cleanup; return 1; }

    log "[warmup ${wd}s]"
    run_load "http://127.0.0.1:9090/lorem.html" "$wd" > /dev/null 2>&1
    log "[warmup done]"

    log "[benchmarking h3 ${d}s: wrk/hey → skipper(:9090) HTTP/1.1 → h3 QUIC → backend(:9980)]"
    run_load "http://127.0.0.1:9090/lorem.html" "$d"

    cleanup
    log "=== h3 done ==="
}

# ---------- main -------------------------------------------------------------

ensure_lorem

case "$proto" in
    h2c) bench_h2c ;;
    h2)  bench_h2  ;;
    h3)  bench_h3  ;;
    all)
        bench_h2c
        bench_h2
        bench_h3
        ;;
    -help|--help|-h)
        log "Usage: benchmark-h2c-h2-h3.sh [h2c|h2|h3|all] [duration_sec] [connections] [warmup_sec] [skipper-binary]"
        log "  example: benchmark-h2c-h2-h3.sh all 12 128 3"
        log "  default: all 12 128 3 ../bin/skipper"
        exit 0
        ;;
    *)
        log "Unknown protocol '$proto'. Use h2c, h2, h3, or all."
        exit 1
        ;;
esac

log; log "[all done]"
