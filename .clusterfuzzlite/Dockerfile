FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:63fdbedeabb5553bdac1362d57a224eb7426fb45a02b0be3c9c7cd3c23897231

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
