#! /bin/bash

if [ "$1" == -help ]; then
	echo benchmark-compress.sh [duration] [warmup-duration]
	exit 0
fi

# duration
d=$1
if [ -z "$d" ]; then d=12; fi

# warmup duration
wd=$2
if [ -z "$wd" ]; then wd=3; fi

cwd=$GOPATH/src/github.com/zalando/skipper/filters/builtin/testing
cd $cwd
go install github.com/zalando/skipper/...
if [ $? -ne 0 ]; then exit -1; fi

pids=

cleanup() {
	kill $pids
}

skp() {
	skipper -access-log-disabled -address $1 -routes-file $2 2> \
		>(grep -v 'write: broken pipe' | \
		grep -v 'write: connection reset by peer' | \
		grep -v 'INFO') &
	pids="$pids $!"
}

ngx() {
	nginx -c $cwd/nginx.conf &
	pids="$pids $!"
}

warmup() {
	wrk -H Accept-Encoding:\ gzip,deflate -c 100 -d "$wd" http://127.0.0.1"$1"/lorem.html | grep -v '^[ \t]'
}

bench() {
	wrk -H Accept-Encoding:\ gzip,deflate -c 100 -d "$d" http://127.0.0.1"$1"/lorem.html
}

trap cleanup SIGINT

echo [starting servers]
skp :9990 static.eskip
ngx
skp :9090 proxy.eskip
echo [servers started, wait 1 sec]
sleep 1

# echo; echo '[warmup]'
warmup :9990
warmup :9080
warmup :9090
# echo '[warmup done]'

echo; echo '[benchmarking nginx]'
bench :9080
echo '[benchmarking nginx done]'

echo; echo '[benchmarking skipper]'
bench :9090
echo '[benchmarking skipper done]'

cleanup
echo; echo '[all done]'
