FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:e2371d49920954bc4208f9b97b55048fdf506dcd2d0a5d6cb62d8148582e8498

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
