FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:3c20b1a4fcd3a901e415a5adc100dd22024901c1132f1710846b9def0fecef1a

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
