FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:71ed90bdf01cd9fad6d508c1d601a4c34b10d9c3f2042808490efae3baba95e1

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
