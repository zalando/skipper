#! /bin/bash

cwd=$GOPATH/src/github.com/zalando/skipper/filters/builtin/testing
cd $cwd
go install github.com/zalando/skipper/...

pids=
cleanup() {
	kill $pids
}

trap cleanup SIGINT

echo; echo [starting servers]
skipper -access-log-disabled -application-log /dev/null -address :9990 -routes-file static.eskip &
pids=$pids" "$(echo $!)

nginx -c $cwd/nginx.conf &
pids=$pids" "$(echo $!)

skipper -access-log-disabled -application-log /dev/null -routes-file proxy.eskip &
pids=$pids" "$(echo $!)
echo; echo [servers started, wait 1 sec]
sleep 1

echo; echo '[warmup]'
ab -H Accept-Encoding:\ gzip,deflate -c 100 -n 10000 http://127.0.0.1:9990/lorem.html 2>&1 > /dev/null | grep -v ^Completed
ab -H Accept-Encoding:\ gzip,deflate -c 100 -n 10000 http://127.0.0.1:9080/lorem.html 2>&1 > /dev/null | grep -v ^Completed
ab -H Accept-Encoding:\ gzip,deflate -c 100 -n 10000 http://127.0.0.1:9090/lorem.html 2>&1 > /dev/null | grep -v ^Completed
echo '[warmup done]'

echo; echo '[benchmarking nginx]'
ab -H Accept-Encoding:\ gzip,deflate -c 100 -n 10000 http://127.0.0.1:9080/lorem.html 2>&1 | grep -v ^Completed
echo '[benchmarking nginx done]'

echo; echo '[benchmarking skipper]'
ab -H Accept-Encoding:\ gzip,deflate -c 100 -n 10000 http://127.0.0.1:9090/lorem.html 2>&1 | grep -v ^Completed
echo '[benchmarking skipper done]'

cleanup
echo; echo '[all done]'
