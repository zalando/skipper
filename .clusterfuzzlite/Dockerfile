FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:91cf0d9c4e82069c6e2bff3c8529ee208cca4c284ac8166d5a8ed179c23c7b47

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
