FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:5fb1c7a2397bcc82393e8cc4f349d68607009c29a5b967db35442d965bb9f10c

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
