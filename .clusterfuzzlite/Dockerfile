FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:972dea510bfa4539b7041fc7b1f5e3868fb70fa23ca0a58df96b420caaab8e6d

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
