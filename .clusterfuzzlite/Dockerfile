FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:66437e689fab89814a5ceeab586d98dd622e63b1ac493c44482e4ae56dc3d23b

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
