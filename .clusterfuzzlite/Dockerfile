FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:c32c6098c3daa9defefffbe5040137fd7cec4b9ed2e5fb85ae1f87cca4ec6623

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
