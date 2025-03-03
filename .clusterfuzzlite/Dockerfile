FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:41601fc0fe28208aa765cdfa45e9d194b6e1e6447be4bf8efd3ccaca67373180

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
