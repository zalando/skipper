FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:b1e41144d9925051d9a0f8be5b2c5a66583063e451080f8965beef972cd597c7

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
