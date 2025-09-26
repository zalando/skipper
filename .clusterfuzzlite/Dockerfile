FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:ecdd7fc47f7fe97e66bc3752c34374cb22e5460551e4b17568155d8c0ffa5316

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
