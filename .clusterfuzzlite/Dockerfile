FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:1d95a720e0e57ae5ed99de73c1329b3d6ce6d3f6646e8d7770df911cdce7f531

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
