FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:54cf78317a403958103759e226f74cc139bf751d622d09af8668dccff201b9b9

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
