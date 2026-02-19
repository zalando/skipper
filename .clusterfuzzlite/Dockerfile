FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:6335e14566776f7eacc2bb39d6e435dfa8c0f310935534fc875e1d29135a8fda

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
