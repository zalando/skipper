FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:76281680003afe5bf5bcdb38667ce467f2772ba3a0e15380809e53a720603918

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
