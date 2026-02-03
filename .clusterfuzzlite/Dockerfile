FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:af693ed057ce43d3143cb470b0ce7b23b723729d24b0d551d2074216d5214202

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
