FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:bab77046ede6fae6f9ab931ada4b17a5adaf71842ac93e32300d06d5b9829891

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
