FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:b196639a9f855fcbfa8882ce08a897344a364a8db359c86d1d5deedc8bbbb006

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
