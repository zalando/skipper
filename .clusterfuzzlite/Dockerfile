FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:4bc04e48690fefa14fe3a4a65715cdd4559bc410ee6ef6a9bbe0d299b4c8ff84

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
