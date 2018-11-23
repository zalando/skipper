# duration
d=$1
if [ -z "$d" ]; then d=12; fi

# concurrency
c=$2
if [ -z "$c" ]; then c=128; fi

# warmup duration
wd=$3
if [ -z "$wd" ]; then wd=3; fi

cwd=$GOPATH/src/github.com/zalando/skipper/skptesting
cd $cwd
GO111MODULE=on go install github.com/zalando/skipper/...
if [ $? -ne 0 ]; then exit -1; fi

loremHead='<!doctype html>
<html>
	<head>
		<meta charset="utf-8">
		<title>Lorem</title>
	</head>
	<body>'
loremP='
		<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore
		et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut
		aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum
		dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui
		officia deserunt mollit anim id est laborum.</p>'
loremTail='
	</body>
</html>'

lorem() {
	echo -n $loremHead > "$cwd"/lorem.html
	for i in $(seq 1 360); do echo -n $loremP >> "$cwd"/lorem.html; done
	echo -n $loremTail >> "$cwd"/lorem.html

	# for nginx:
	mkdir -p /tmp/lorem
	cp "$cwd"/lorem.html /tmp/lorem
}

pids=

cleanup() {
	kill $pids
}

cleanup-exit() {
	cleanup
	exit 0
}

skp() {
	tlskey=
	tlscert=
	if [ "$3" = tls ]; then
		tlskey=key.pem
		tlscert=cert.pem
	fi

	skipper -access-log-disabled -address "$1" -routes-file "$cwd"/"$2" -insecure \
		-idle-conns-num "$c" -close-idle-conns-period 3 \
		-tls-key "$tlskey" -tls-cert "$tlscert" 2> \
		>(grep -Ev 'write: broken pipe|connection reset by peer|getsockopt: connection refused|INFO') &
	pids="$pids $!"
}

skp-pprof() {
	skptesting "$cwd"/cpu-profile.prof "$cwd"/mem-profile.prof "$1" "$2" 2> \
		-idle-conns-num "$c" -close-idle-conns-period 3 \
		>(grep -Ev 'write: broken pipe|EOF|write: connection reset by peer|getsockopt: connection refused' | \
		grep -Ev 'connect: cannot assign requested address|read: connection reset by peer|INFO') &
	pids="$pids $!"
}

ngx() {
	nginx -c "$cwd"/"$1" 2> \
		>(grep -v '[(]111: Connection refused[)]') &
	pids="$pids $!"
}

warmup() {
	protocol=http
	[ "$3" = tls ] && protocol=https
	wrk -H "$2" -c "$c" -d "$wd" "$protocol"://127.0.0.1"$1"/lorem.html | grep -v '^[ \t]'
}

bench() {
	protocol=http
	[ "$3" = tls ] && protocol=https
	wrk -H "$2" -c "$c" -d "$d" "$protocol"://127.0.0.1"$1"/lorem.html
}

log() {
	echo $@ >&2
}

check_deps() {
  which wrk &>/dev/null
  if [ $? -eq 1 ]
  then
    echo "ERR: dependency 'wrk' https://github.com/wg/wrk not found in \$PATH"
    exit 2
  fi
}
