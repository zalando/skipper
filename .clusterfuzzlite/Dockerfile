FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:85f6664e8ff542b48592eee56b2c0b98b50332b8430e920826324c4a367111f8

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
