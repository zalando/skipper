FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:9121caae4e215b8cac6c9ba88c64db3884f6f74055bbf480241e27fd62d44358

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
