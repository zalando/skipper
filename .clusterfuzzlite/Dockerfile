FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:94a689e7ec86efec1603bb3beab6f585bbef293809d6074a7d14920dc55c58a0

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
