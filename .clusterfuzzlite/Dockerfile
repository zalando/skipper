FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:08c1570f496db8185c53986230e36d9c3db52982fbbbd795d8c8aed1a0bcbd1b

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
