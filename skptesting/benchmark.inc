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
go install github.com/zalando/skipper/...
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

skp() {
	tlskey=
	tlscert=
	if [ "$3" = tls ]; then
		tlskey=key.pem
		tlscert=cert.pem
	fi

	skipper -access-log-disabled -address "$1" -routes-file "$cwd"/"$2" -insecure \
		-idle-conns "$c" -close-idle-conns 3 \
		-tls-key "$tlskey" -tls-cert "$tlscert" 2> \
		>(grep -v 'write: broken pipe' | \
		grep -v 'connection reset by peer' | \
		grep -v 'getsockopt: connection refused' | \
		grep -v 'INFO') &
	pids="$pids $!"
}

skp-pprof() {
	skptesting "$cwd"/cpu-profile.prof "$cwd"/mem-profile.prof "$1" "$2" 2> \
		-idle-conns "$c" -close-idle-conns 3 \
		>(grep -v 'write: broken pipe' | \
		grep -v 'EOF' | \
		grep -v 'write: connection reset by peer' | \
		grep -v 'getsockopt: connection refused' | \
		grep -v 'connect: cannot assign requested address' | \
		grep -v 'read: connection reset by peer' | \
		grep -v 'INFO') &
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
