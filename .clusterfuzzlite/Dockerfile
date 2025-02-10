FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:7031d0f13d7cbd86a1c9a5807bb83196f3614c9c1631a0d34174bcbe6b9c12c3

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
