FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:bfc4761e36a60fc2abe1c20aa4949d53514915e9d14cdd9565b981370b3ccbca

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
