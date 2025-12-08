FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:b0571a17ec7619e712d8eeec459b7a4f4516bcbd18c1c9ed33209cad365dcef2

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
