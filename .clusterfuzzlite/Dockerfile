FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:fd6832a7a2b20111b2b792ac71184e9684c84652590f136141072a982e0fc26f

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
