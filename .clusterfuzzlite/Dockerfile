FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:b365850884c556d66a55ffa5d56c198079ff28afef714cf47cce07c4841bbd42

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
