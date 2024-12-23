FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:ae768e54b0baca5d9ad4deeca8a5daa8375538aa9c39be6e5f02b71651345b3e

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
