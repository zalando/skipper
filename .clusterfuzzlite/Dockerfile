FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:74c2133a1fc6ec6c90270998c1156361cde70a2ef91531d4485a4ed59e5e1618

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
