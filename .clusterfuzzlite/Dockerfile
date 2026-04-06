FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:c07cb46ba5f1a5d16f9318cfb2eeac784d4f26bb4f2ce75b91cf84dd516c0f60

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
