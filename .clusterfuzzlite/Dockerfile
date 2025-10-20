FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:19e7491c0a8e8d984e37eaa56a8c030f54109a81ae30727cb292517f6a77c1cd

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
