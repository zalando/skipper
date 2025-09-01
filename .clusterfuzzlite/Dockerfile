FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:bbbd2b30fdf5d32bf1082c10d2a66994bec3d4aa104de5ff464a859fa264bbfe

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
