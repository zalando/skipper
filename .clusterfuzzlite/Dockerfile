FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:2e968188b9dc32a3753304a889ec5b1c2ece17656608d4539dce3cd47a13480f

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
