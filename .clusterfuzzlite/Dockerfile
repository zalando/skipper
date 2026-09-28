FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:db46af0d66c297dce2be95304f7a12c4e0f2901fbe21f2fc10a1de19d06c5cf9

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
