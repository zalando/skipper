#! /bin/bash

log() {
	echo $@ >&2
}

# take subcommand, default 'routes'
cmd=routes
if [ -n "$1" ]; then cmd=$1; fi
shift

# take flags, countdown for positional
flagCount=0
while getopts "h:s:n:p:P:F:E:" arg; do
	eval $arg=\'"$OPTARG"\'
	flagCount=$(expr $flagCount + 2)
done
if [ $flagCount -gt 0 ]; then
	for (( i=0; i<$flagCount; i++ )); do shift; done
fi

# host or default
host=${h:-http://localhost:9080}

# output json selector, default all
select=${s:-.}

auth=Authorization:\ Bearer\ token-user~1-employees-route
ctype=Content-Type:\ application/json

now() {
	echo \"$(date -u +%Y-%m-%dT%H:%M:%S)\"
}

# expecting path
get() {
	curl -sH "$auth".read "$host"/"$1"
}

# expecting path and payload
put() {
	curl -sH "$auth".write "$host"/"$1" -H "$ctype" -d "$2"
}

# expecting path
delete() {
	curl -X DELETE -sH "$auth".write "$host"/"$1"
}

routes() {
	get routes
}

current-routes() {
	get current-routes
}

updated-routes() {
	timestamp="$1"
	if [ -z $timestamp ]; then
		timestamp=$(now)
	fi

	# strip quotes
	timestamp=$(echo $timestamp | sed -e 's/^"//' -e 's/"$//')

	get updated-routes/$timestamp
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

# expecting name(n), path(p) and endpoint(E) flags
# optionally predicates(P) and filters(F) flags
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

	endpoint="$E"
	if [ -z "$endpoint" ]; then
		log missing endpoint
		exit 1
	fi

	predicates=${P:-"[]"}
	filters=${F:-"[]"}

	put routes '{
		"name": "'"$name"'",
		"activate_at": '$(now)',
		"path_id": '"$path"',
		"uses_common_filters": true,
		"endpoint": "'"$endpoint"'",
		"predicates": '"$predicates"',
		"filters": '"$filters"'
	}'
}

# expecting route id
delete-route() {
	id="$1"
	if [ -z "$id" ]; then
		log missing id
	fi

	delete routes/"$id"
}

# call subcommand and print json
$cmd $@ | jq --monochrome-output "$select"
