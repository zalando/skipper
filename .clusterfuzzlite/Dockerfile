FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:9bf7fad8ca02443224c7518392d80c97a62b8cb0822f03aadf9193a7e27346f0

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
