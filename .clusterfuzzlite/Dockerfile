FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:fc4d4f4fd1a2c4cabb850f69cd43a93c03d3155fe2a5ec238330ef6295d3ce9f

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
