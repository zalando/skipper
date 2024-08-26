FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:74a18c1f123123003408451656a487ec52915f0e721b669de4fe0b3aac51d6b2

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
