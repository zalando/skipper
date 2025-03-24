FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:5890e8b6e3a24c325d7d51c2adb5f6140ebb8a07ea1d271bfac0f8cb47ddf82a

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
