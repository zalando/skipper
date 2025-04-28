FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:918701ddb6b05b78fc5a403727868e97926b3bc3d8c884dac810a8c9655487a8

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
