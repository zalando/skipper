FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:76c50f3facb7396241e91c6da526dcc02d4e6459cc822acac034a4fb68b49d53

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
