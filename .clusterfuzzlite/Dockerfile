FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:f4b55824218c62ca44a0fb3985c2ca528c3fd64d52987b72ce45b5ec520123dd

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
