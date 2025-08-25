FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:5ba0fcded8f66ca14d2dbbe6f10d8a7ecf17eda78439eb866d2d32925e613cf0

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
