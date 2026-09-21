FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:b0087029454cec495da9f57e365628143bdb1b56eb180873fa5e50699d2f2b6e

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
