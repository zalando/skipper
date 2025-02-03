FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:8cacfff023ee3485afb059b2a14d1615e6059f27b7a4f465ec8a5b85d26156b7

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
