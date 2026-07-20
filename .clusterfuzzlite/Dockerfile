FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:848ba220d07ed8ff3ce7f2d73af208c75c34d9f18b955b0702cfc9b25527a85e

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
