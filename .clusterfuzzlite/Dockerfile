FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:679783b8b7a36560a9d41d66f1f25c3036e6b5cae1088f88a0357384e6757ec4

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
