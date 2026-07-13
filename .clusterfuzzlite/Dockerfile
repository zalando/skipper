FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:4727e695a33e821967c44406449493a3675657cf3dec091d12f7b2fda1e93186

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
