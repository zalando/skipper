FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:da10193c2cac6f561d00ae38f407060ecb6900b9c02e9e8d24aaa971242aa8d3

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
