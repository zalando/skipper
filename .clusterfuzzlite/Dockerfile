FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:4103d032c0ec6fa020c0cc46fcebd6948c7b1815591c5e6daae4b8baa8e750a1

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
