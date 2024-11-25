FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:bb908eb135fa1667d7cec0214ad975592827a1de5a0e4ca4bdc2ef0ec067b6fc

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
