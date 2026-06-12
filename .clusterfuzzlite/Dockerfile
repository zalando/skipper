FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:bf2c8e9f73e2ced735dce88a2122f73063d71298d2f73935541283905755648c

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
