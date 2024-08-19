FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:efaf3b6257ce51b56c74b2ce7c1d8165674febdc5e43859904c208f11e581482

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
