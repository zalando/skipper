FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:9e76082f0d5e86f349c9f87f4e9bc994f905ba865160886ed121fe661e59d4aa

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
