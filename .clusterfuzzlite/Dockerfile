FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:a1da66b510931c89063ecf9ab5f914e3cee7898dd90bdf3d31a246bc07e72c5b

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
