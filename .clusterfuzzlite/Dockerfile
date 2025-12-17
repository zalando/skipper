FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:8d47a36a59e9dabfa51eafb5762cd48c15f816d6ed8b3f99ea39b373ad83b137

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
