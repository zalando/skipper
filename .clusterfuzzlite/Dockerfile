FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:77ecf36140442b793d3ba70b1cbb5694132726c28ec2c19b1b8908b2c6266e17

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
