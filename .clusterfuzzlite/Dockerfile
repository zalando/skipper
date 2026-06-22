FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:ffa525743760f16e47d81051344c79c3f34c673aeebaf01f6afe7608530de68a

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
