FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:5f8a362c0f8972d0d3d73e7b56258b4f22152ddf54a0450428b05fec0a6dd2f8

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
