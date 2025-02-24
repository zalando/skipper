FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:d354bdf1a0843e869593a7030accd20a6e9a964fe699a010f8d671a8914ad448

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
