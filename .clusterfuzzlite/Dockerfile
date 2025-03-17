FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:54add4dd7c5c1eefd7cfde9f418a3bce520f48d41141a9d150f2e1953b34d3ee

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
