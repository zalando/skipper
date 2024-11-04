FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:548617f0f4ebb9b5c30397560de93d32531b8e84a8c4228adb0cd1571f4087c8

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
