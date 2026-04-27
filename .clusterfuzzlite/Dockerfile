FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:f51f458733d008ce8d21cf50636262e44ec6fd096f3c396a951a321da0f3163c

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
