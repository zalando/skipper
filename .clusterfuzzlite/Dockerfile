FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:35e834b55ad9dff1bdfba7647e71df0ca1b888cb63d98366495c539002b09970

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
