FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:9c585d0d258966044f98638fc18812b007400ca14c6a610a92af2aade12f3cb6

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
