FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:f7970e735dcd3c5f360ae92b40a20414d70ef3b534ff6f3566dcad1b1cb2ec4c

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
