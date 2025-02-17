FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:98616f371934cbdd901131b29a36cd636ddcd407aa622d9a29bd96f1d2631630

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
