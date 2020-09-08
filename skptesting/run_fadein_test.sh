#!/bin/bash

function run_test() {
  GO111MODULE=on go test ./loadbalancer -run="$1" -count=1 -v | awk '/fadein_test.go:[0-9]+: CSV/ {print substr($0, index($0,$4)) }'
}

cwd=$( dirname "${BASH_SOURCE[0]}" )

if [ -z "${1+x}" ]
then
  echo "$0 <test> [<test>...]"
  echo "Example:"
  echo "$0 TestFadeIn/4th TestFadeIn/3rd"
else
  d=$(mktemp -d)
  for t in "$@"
  do
    file="$d/${t##*/}.csv"
    out="${t%/*}_${t##*/}.png"
    echo "$t has csv input file: $file and output file: $out"
    run_test "$t" > "$file"
    "./$cwd/analyze_fadein.r" --file "$file" --title "$t" --output "$out"
  done
fi
