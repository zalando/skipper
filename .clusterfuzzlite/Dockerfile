FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:b71e8970c4e33a9c5bdaf4054b6b6241268a11cd308d76a3574910da2b668c8a

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
