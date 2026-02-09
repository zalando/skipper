FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:bb3b16c83cc140211513db1690b142dd5858d6b73e5ce0e902faf64cdaa368a6

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
