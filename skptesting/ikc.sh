#! /bin/bash

log() {
	echo $@ >&2
}

cmd=routes
if [ -n "$1" ]; then cmd=$1; fi
shift

flagCount=0
while getopts "h:s:n:p:" arg; do
	eval $arg="$OPTARG"
	flagCount=$(expr $flagCount + 2)
done
if [ $flagCount -gt 0 ]; then
	for (( i=0; i<$flagCount; i++ )); do shift; done
fi
host=${h:-http://localhost:9080}
select=${s:-.}

auth=Authorization:\ Bearer\ token-user~1-employees-route
ctype=Content-Type:\ application/json

now() {
	date -u +%Y-%m-%dT%H:%M:%S
}

get() {
	curl -sH "$auth".read "$host"/"$1"
}

put() {
	curl -sH "$auth".write "$host"/"$1" -H Content-Type:\ application/json -d "$2"
}

routes() {
	get routes
}

paths() {
	get paths
}

hosts() {
	get hosts
}

hostids() {
	hosts | jq --compact-output '[.[] | .id]'
}

mkpath() {
	put paths '{"uri": "'$1'", "host_ids": '"$(hostids)"'}'
}

mkroute() {
	name="$n"
	if [ -z "$name" ]; then
		log missing name
		exit 1
	fi

	path="$p"
	if [ -z "$path" ]; then
		log missing path
		exit 1
	fi

	echo "$1"
	echo "$@"
	predicates="$@"
	if [ -z "$predicates" ]; then
		predicates='[]'
	fi

	# put routes '{
	# 	"name": "'$name'",
	# 	"activate_at": "'$(now)'",
	# 	"path_id": '$path',
	# 	"uses_common_filters": true,
	# 	"route": {}
	# }'
}

$cmd $@ | jq --monochrome-output $select
