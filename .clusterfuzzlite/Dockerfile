FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:2347618de7ba036a4822540f562636b6fb06c804be5381de24e2d3b2d5d70207

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
