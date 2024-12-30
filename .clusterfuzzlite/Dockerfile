FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:9a47ebe4dc1d194ed95889081b5802d77616151da508cb1f62f7857d7f3c4f00

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
