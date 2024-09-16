FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:130bed23f6e9f8f09963a4165d814b66164967c300409aeb8a6f6db48b6783ff

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
