FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:c9d6cfc40344db3c96498cfc285554ec4f11623a7ecc29dabbcb56fd00fc4a9c

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
