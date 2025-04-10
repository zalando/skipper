FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:47807310f889d99676cc1250ec3b9dfb478d033e8347d305daf96fb2a7061ffd

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
